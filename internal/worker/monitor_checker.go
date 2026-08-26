package worker

import (
	"context"
	"sync"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/rs/zerolog"
)

// MonitorSupervisor mengelola satu goroutine + ticker per monitor aktif
// (keputusan arsitektur: goroutine-per-monitor, bukan ticker global —
// trade-off-nya lifecycle management di file ini jadi lebih kompleks,
// tapi presisi scheduling sesuai interval_sec masing-masing monitor).
type MonitorSupervisor struct {
	monitorRepo domain.MonitorRepository
	checker     *usecase.MonitorCheckerUsecase
	logger      zerolog.Logger

	mu      sync.Mutex
	cancels map[string]context.CancelFunc
}

func NewMonitorSupervisor(
	monitorRepo domain.MonitorRepository,
	checker *usecase.MonitorCheckerUsecase,
	logger zerolog.Logger,
) *MonitorSupervisor {
	return &MonitorSupervisor{
		monitorRepo: monitorRepo,
		checker:     checker,
		logger:      logger,
		cancels:     make(map[string]context.CancelFunc),
	}
}

// Run dipanggil sekali dari cmd/worker/main.go. Rebuild semua goroutine
// checker dari state DB saat startup (lihat komentar
// MonitorRepository.ListAll di domain/monitor.go — worker restart tidak
// boleh asumsi state lama masih ada), block sampai ctx dibatalkan, lalu
// cancel semua goroutine anak.
func (s *MonitorSupervisor) Run(ctx context.Context) error {
	monitors, err := s.monitorRepo.ListAll(ctx)
	if err != nil {
		return err
	}

	s.logger.Info().Int("count", len(monitors)).Msg("monitor supervisor: rebuilding checkers from db")
	for _, m := range monitors {
		s.spawn(ctx, m)
	}

	<-ctx.Done()

	s.mu.Lock()
	for _, cancel := range s.cancels {
		cancel()
	}
	s.mu.Unlock()

	return ctx.Err()
}

// spawn start goroutine baru buat 1 monitor. Kalau monitor.ID sudah
// punya goroutine jalan (kasus Sync ulang setelah update), goroutine
// lama di-cancel dulu — mencegah 2 ticker jalan bareng buat monitor yang
// sama.
func (s *MonitorSupervisor) spawn(parentCtx context.Context, m *domain.Monitor) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, exists := s.cancels[m.ID]; exists {
		cancel()
	}

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancels[m.ID] = cancel

	go s.runChecker(ctx, m)
}

func (s *MonitorSupervisor) runChecker(ctx context.Context, m *domain.Monitor) {
	ticker := time.NewTicker(time.Duration(m.IntervalSec) * time.Second)
	defer ticker.Stop()

	s.logger.Info().Str("monitor_id", m.ID).Str("url", m.URL).Int("interval_sec", m.IntervalSec).Msg("monitor checker started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check, err := s.checker.Check(ctx, m)
			if err != nil {
				s.logger.Error().Err(err).Str("monitor_id", m.ID).Msg("monitor check failed")
				continue
			}
			// Debug level (bukan Info) — ini bakal muncul TIAP TICK
			// buat SEMUA monitor, jadi kalau default log level Info,
			// ini otomatis "diam" di production tanpa perlu ubah kode
			// lagi. Nyalain Debug pas lagi troubleshooting spesifik.
			s.logger.Debug().
				Str("monitor_id", m.ID).
				Int("status_code", check.StatusCode).
				Int("latency_ms", check.LatencyMs).
				Bool("is_up", check.IsUp).
				Msg("monitor check completed")
		}
	}
}

// Sync dipanggil monitor_sync.go saat terima event monitor.created /
// monitor.updated. Fetch state TERBARU dari DB (bukan trust payload
// event yang cuma berisi ID) — hindari state basi kalau event datang
// telat atau ke-reorder.
func (s *MonitorSupervisor) Sync(ctx context.Context, monitorID string) {
	m, err := s.monitorRepo.GetByID(ctx, monitorID)
	if err != nil {
		// Kemungkinan monitor sudah dihapus lagi sebelum event ini
		// sempat diproses — race kecil yang wajar di sistem async,
		// bukan error fatal.
		s.logger.Warn().Err(err).Str("monitor_id", monitorID).Msg("monitor supervisor: sync failed, monitor may no longer exist")
		return
	}
	s.spawn(ctx, m)
}

// Remove dipanggil monitor_sync.go saat terima event monitor.deleted.
func (s *MonitorSupervisor) Remove(monitorID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if cancel, exists := s.cancels[monitorID]; exists {
		cancel()
		delete(s.cancels, monitorID)
	}
}

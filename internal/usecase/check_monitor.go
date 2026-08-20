package usecase

import (
	"context"
	"net/http"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// MonitorNotifier didefinisikan terpisah dari Notifier (evaluate_alert.go)
// — signature beda total (Monitor, bukan AlertRule+Issue), Interface
// Segregation (lihat penjelasan di notifier.go).
type MonitorNotifier interface {
	NotifyMonitorDown(ctx context.Context, monitor *domain.Monitor, consecutiveFailures int) error
}

// checkerUserAgent — custom UA biar WAF/CDN tidak nge-block request
// checker sebagai bot generic (lihat diskusi keputusan HTTP client).
const checkerUserAgent = "SentinelIX-Uptime-Monitor/1.0"

// monitorStatusChangedEvent — nama event WS sesuai 04-API-DESIGN.md §8.
const monitorStatusChangedEvent = "monitor.status_changed"

type MonitorCheckerUsecase struct {
	monitorRepo 		domain.MonitorRepository
	monitorCheckRepo 	domain.MonitorCheckRepository
	notifier 			MonitorNotifier
	broadcaster			MonitorSyncPublisher
	httpClient			*http.Client
}

func NewMonitorCheckerUsecase(
	monitorRepo domain.MonitorRepository,
	monitorCheckRepo domain.MonitorCheckRepository,
	notifier MonitorNotifier,
	broadcaster MonitorSyncPublisher,
) *MonitorCheckerUsecase {
	return &MonitorCheckerUsecase{
		monitorRepo:		monitorRepo,
		monitorCheckRepo: 	monitorCheckRepo,
		notifier: 			notifier,
		broadcaster: 		broadcaster,
		httpClient: 		&http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Check melakukan satu kali ping ke monitor.URL, simpan hasilnya, evaluasi
// apakah status monitor perlu berubah, lalu return record MonitorCheck
// yang baru dibuat — dipakai caller (worker/monitor_checker.go) buat
// logging observability, TANPA perlu query ulang ke DB.
func (uc *MonitorCheckerUsecase) Check(ctx context.Context, monitor *domain.Monitor) (*domain.MonitorCheck, error) {
	statusCode, latencyMs, isUp := uc.ping(ctx, monitor.URL)

	check := &domain.MonitorCheck{
		MonitorID: 		monitor.ID,
		StatusCode: 	statusCode,
		LatencyMs:		latencyMs,
		IsUp: 			isUp,
	}
	if err := uc.monitorCheckRepo.Create(ctx, check); err != nil {
		return nil, err
	}

	newStatus, err := uc.evaluateStatus(ctx, monitor, isUp)
	if err != nil {
		return check, err
	}

	if newStatus != monitor.Status {
		if err := uc.monitorRepo.UpdateStatus(ctx, monitor.ID, newStatus); err != nil {
			return check, err
		}
		monitor.Status = newStatus

		_ = uc.broadcaster.Publish(ctx, monitor.ProjectID, monitorStatusChangedEvent, map[string]interface{}{
			"monitor_id": 	monitor.ID,
			"is_up":		newStatus == domain.MonitorStatusUp,
		})

		if newStatus == domain.MonitorStatusDown {
			if err := uc.notifier.NotifyMonitorDown(ctx, monitor, monitor.FailureThreshold); err != nil {
				return check, err
			}
		}
	}

	return check, nil
}

// evaluateStatus: naik ke "up" itu IMMEDIATE (1 check sukses cukup,
// konsisten dengan perilaku standar uptime monitor — recovery harus
// cepat ke-detect). Turun ke "down" butuh FailureThreshold check
// TERAKHIR semuanya gagal (FR-19: consecutive, bukan windowed count).
// Kalau belum cukup consecutive failure, status TIDAK berubah dulu.
func (uc *MonitorCheckerUsecase) evaluateStatus(ctx context.Context, monitor *domain.Monitor, isUp bool) (string, error) {
	if isUp {
		return domain.MonitorStatusUp, nil
	}

	recent, err := uc.monitorCheckRepo.ListRecentByMonitorID(ctx, monitor.ID, monitor.FailureThreshold)
	if err != nil {
		return "", err
	}
	if len(recent) < monitor.FailureThreshold {
		return monitor.Status, nil
	}
	for _, c := range recent {
		if c.IsUp {
			return monitor.Status, nil
		}
	}
	return domain.MonitorStatusDown, nil
}

// ping melakukan HTTP GET, ukur latency, tentukan is_up. Network error
// (timeout, connection refused, DNS failure, dll) dianggap is_up=false
// dengan status_code=0 — checker TIDAK return error buat kasus ini,
// karena "endpoint down" itu HASIL yang valid buat dicatat, bukan error
// internal sistem kita.
func (uc *MonitorCheckerUsecase) ping(ctx context.Context, targetURL string) (statusCode, latencyMs int, isUp bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, 0, false
	}
	req.Header.Set("User-Agent", checkerUserAgent)

	start := time.Now()
	resp, err := uc.httpClient.Do(req)
	elapsed := time.Since(start)

	if err != nil {
		return 0, int(elapsed.Milliseconds()), false
	}
	defer resp.Body.Close()

	return resp.StatusCode, int(elapsed.Milliseconds()), resp.StatusCode < 400
}
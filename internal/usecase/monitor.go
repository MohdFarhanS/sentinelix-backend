package usecase

import (
	"context"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// MonitorSyncPublisher memberi tahu cmd/worker bahwa monitor berubah
// (created/updated/deleted) — reuse Redis Pub/Sub yang sama dengan WS
// broadcast. Signature-nya SENGAJA sama persis dengan
// redisrepo.Broadcaster.Publish, jadi *redisrepo.Broadcaster otomatis
// satisfy interface ini tanpa perlu adapter (Go structural typing).
//
// PENTING: event type (domain.MonitorSyncCreated dkk) didefinisikan di
// domain, BUKAN di internal/worker — kalau didefinisikan di worker,
// usecase/monitor.go harus import internal/worker, padahal
// internal/worker sendiri import internal/usecase (MonitorSupervisor
// butuh *usecase.MonitorCheckerUsecase). Itu import cycle, tidak akan
// compile. domain adalah layer paling dalam, aman diimport siapa saja.
type MonitorSyncPublisher interface {
	Publish(ctx context.Context, projectID, eventType string, data interface{}) error
}

type MonitorUsecase struct {
	monitorRepo      domain.MonitorRepository
	monitorCheckRepo domain.MonitorCheckRepository
	projectRepo      domain.ProjectRepository
	syncPublisher    MonitorSyncPublisher
}

func NewMonitorUsecase(
	monitorRepo domain.MonitorRepository,
	monitorCheckRepo domain.MonitorCheckRepository,
	projectRepo domain.ProjectRepository,
	syncPublisher MonitorSyncPublisher,
) *MonitorUsecase {
	return &MonitorUsecase{
		monitorRepo:      monitorRepo,
		monitorCheckRepo: monitorCheckRepo,
		projectRepo:      projectRepo,
		syncPublisher:    syncPublisher,
	}
}

type CreateMonitorInput struct {
	UserID           string
	ProjectID        string
	URL              string
	Name             string // opsional — kosong = fallback ke URL saat ditampilkan (domain.Monitor.DisplayName())
	IntervalSec      int
	Channel          string
	ChannelTarget    string
	FailureThreshold int
}

// Create — ownership check dulu (403 sebelum 400), pola sama seperti
// AlertRuleUsecase.Create. Default IntervalSec/FailureThreshold TIDAK
// diisi di sini — itu tanggung jawab handler (konsisten dengan pola
// cooldown_minutes default 60 di handler_alert.go, bukan di usecase).
func (uc *MonitorUsecase) Create(ctx context.Context, in CreateMonitorInput) (*domain.Monitor, error) {
	project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != in.UserID {
		return nil, ErrForbidden
	}

	monitor := &domain.Monitor{
		ProjectID:        in.ProjectID,
		URL:              in.URL,
		Name:             in.Name,
		IntervalSec:      in.IntervalSec,
		Channel:          in.Channel,
		ChannelTarget:    in.ChannelTarget,
		FailureThreshold: in.FailureThreshold,
	}
	if err := monitor.Validate(); err != nil {
		return nil, err
	}

	if err := uc.monitorRepo.Create(ctx, monitor); err != nil {
		return nil, err
	}

	// projectID dikosongkan sengaja — bukan concern WS routing di sini,
	// cuma MonitorSupervisor yang peduli event ini (filter Type di
	// monitor_sync.go). Publish error sengaja di-swallow (bukan
	// menggagalkan Create) — monitor sudah tersimpan di DB, kegagalan
	// notify sync itu tidak boleh bikin API call gagal buat user;
	// worst case MonitorSupervisor baru pickup monitor ini pas restart
	// berikutnya (ListAll rebuild).
	_ = uc.syncPublisher.Publish(ctx, "", domain.MonitorSyncCreated, map[string]string{"id": monitor.ID})

	return monitor, nil
}

type ListMonitorsInput struct {
	UserID    string
	ProjectID string
}

func (uc *MonitorUsecase) List(ctx context.Context, in ListMonitorsInput) ([]*domain.Monitor, error) {
	project, err := uc.projectRepo.GetByID(ctx, in.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != in.UserID {
		return nil, ErrForbidden
	}

	return uc.monitorRepo.ListByProjectID(ctx, in.ProjectID)
}

// getOwnedMonitor — pola sama persis getOwnedIssue/getOwnedAlertRule.
func (uc *MonitorUsecase) getOwnedMonitor(ctx context.Context, userID, monitorID string) (*domain.Monitor, error) {
	monitor, err := uc.monitorRepo.GetByID(ctx, monitorID)
	if err != nil {
		return nil, err
	}

	project, err := uc.projectRepo.GetByID(ctx, monitor.ProjectID)
	if err != nil {
		return nil, err
	}
	if project.UserID != userID {
		return nil, ErrForbidden
	}

	return monitor, nil
}

func (uc *MonitorUsecase) GetByID(ctx context.Context, userID, monitorID string) (*domain.Monitor, error) {
	return uc.getOwnedMonitor(ctx, userID, monitorID)
}

type UpdateMonitorInput struct {
	UserID           string
	MonitorID        string
	URL              *string
	Name             *string
	IntervalSec      *int
	Channel          *string
	ChannelTarget    *string
	FailureThreshold *int
}

func (uc *MonitorUsecase) Update(ctx context.Context, in UpdateMonitorInput) (*domain.Monitor, error) {
	monitor, err := uc.getOwnedMonitor(ctx, in.UserID, in.MonitorID)
	if err != nil {
		return nil, err
	}

	if in.URL != nil {
		monitor.URL = *in.URL
	}
	if in.Name != nil {
		monitor.Name = *in.Name
	}
	if in.IntervalSec != nil {
		monitor.IntervalSec = *in.IntervalSec
	}
	if in.Channel != nil {
		monitor.Channel = *in.Channel
	}
	if in.ChannelTarget != nil {
		monitor.ChannelTarget = *in.ChannelTarget
	}
	if in.FailureThreshold != nil {
		monitor.FailureThreshold = *in.FailureThreshold
	}

	if err := monitor.Validate(); err != nil {
		return nil, err
	}

	if err := uc.monitorRepo.Update(ctx, monitor); err != nil {
		return nil, err
	}

	_ = uc.syncPublisher.Publish(ctx, "", domain.MonitorSyncUpdated, map[string]string{"id": monitor.ID})

	return monitor, nil
}

type DeleteMonitorInput struct {
	UserID    string
	MonitorID string
}

func (uc *MonitorUsecase) Delete(ctx context.Context, in DeleteMonitorInput) error {
	if _, err := uc.getOwnedMonitor(ctx, in.UserID, in.MonitorID); err != nil {
		return err
	}

	if err := uc.monitorRepo.Delete(ctx, in.MonitorID); err != nil {
		return err
	}

	_ = uc.syncPublisher.Publish(ctx, "", domain.MonitorSyncDeleted, map[string]string{"id": in.MonitorID})

	return nil
}

type ListMonitorChecksInput struct {
	UserID    string
	MonitorID string
	From      *time.Time
	To        *time.Time
}

// ListChecks dipakai GET /monitors/:id/checks (chart uptime dashboard).
// Taruh di MonitorUsecase (bukan MonitorCheckerUsecase) karena ini
// concern-nya dashboard read, bukan checker logic — MonitorCheckerUsecase
// murni soal eksekusi ping + evaluasi status.
func (uc *MonitorUsecase) ListChecks(ctx context.Context, in ListMonitorChecksInput) ([]*domain.MonitorCheck, error) {
	if _, err := uc.getOwnedMonitor(ctx, in.UserID, in.MonitorID); err != nil {
		return nil, err
	}
	return uc.monitorCheckRepo.ListByMonitorID(ctx, in.MonitorID, in.From, in.To)
}

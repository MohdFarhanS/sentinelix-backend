package domain

import (
	"context"
	"errors"
	"time"
)

// Kondisi & channel yang valid, dipakai di Validate() dan di usecase
// evaluate_alert.go biar tidak ada string literal tersebar.
const (
	ConditionTypeNewIssue  = "new_issue"
	ConditionTypeThreshold = "threshold"

	ChannelEmail = "email"
	ChannelSlack = "slack"
)

// AlertRule merepresentasikan kondisi kapan notifikasi harus dikirim untuk
// suatu project. ConditionType menentukan mekanisme evaluasi yang dipakai
// (lihat internal/usecase/evaluate_alert.go):
//   - "new_issue": dicek event-driven, langsung setelah issue baru tergrup
//   - "threshold": dicek periodic ticker, WindowMinutes dipakai buat
//     hitung count event per issue dalam rentang waktu tsb (windowed,
//     BUKAN issues.count yang kumulatif).
type AlertRule struct {
	ID              string
	ProjectID       string
	ConditionType   string
	Threshold       int
	WindowMinutes   int
	CooldownMinutes int
	Channel         string
	ChannelTarget   string
	CreatedAt       time.Time
}

var (
	ErrAlertRuleNotFound           = errors.New("alert rule not found")
	ErrAlertConditionTypeInvalid   = errors.New("conditon_type must be 'new_issue' or 'threshold'")
	ErrAlertChannelInvalid         = errors.New("channel must be 'email' or 'slack'")
	ErrAlertThresholdInvalid       = errors.New("threshold must be > 0 for condition_type 'threshold'")
	ErrAlertWindowMinutesInvalid   = errors.New("window_minutes must be > 0")
	ErrAlertCooldownMinutesInvalid = errors.New("cooldown_minutes must be > 0")
	ErrAlertChannelTargetRequired  = errors.New("channel_target is required")
)

// Validate menjaga integritas data sebelum masuk ke repository. Threshold
// > 0 cuma wajib untuk condition_type "threshold" — untuk "new_issue",
// field Threshold tidak dipakai sama sekali di evaluasi.
func (r *AlertRule) Validate() error {
	if r.ConditionType != ConditionTypeNewIssue && r.ConditionType != ConditionTypeThreshold {
		return ErrAlertConditionTypeInvalid
	}
	if r.Channel != ChannelEmail && r.Channel != ChannelSlack {
		return ErrAlertChannelInvalid
	}
	if r.ChannelTarget == "" {
		return ErrAlertChannelTargetRequired
	}
	if r.ConditionType == ConditionTypeThreshold && r.WindowMinutes <= 0 {
		return ErrAlertWindowMinutesInvalid
	}
	if r.CooldownMinutes <= 0 {
		return ErrAlertCooldownMinutesInvalid
	}
	return nil
}

// AlertLog mencatat histori pengiriman notifikasi — jadi source of truth
// cooldown (lihat GetLastSentAt), BUKAN Redis TTL key (keputusan Sprint 6,
// konsisten dgn dedup_cache Sprint 3: atomic DB query sudah cukup).
// Granularitas per (AlertRuleID, IssueID): issue lain yang exceed
// threshold pada rule yang sama TIDAK ikut ke-cooldown oleh issue lain.
type AlertLog struct {
	ID          string
	AlertRuleID string
	IssueID     string
	SentAt      time.Time
}

// AlertRuleRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres.
type AlertRuleRepository interface {
	// Create insert alert rule baru. r.ID dan r.CreatedAt di-generate DB,
	// di-scan balik ke pointer r (pola sama seperti ProjectRepository.Create).
	Create(ctx context.Context, r *AlertRule) error

	// GetByID dipakai buat cek ownership (lewat r.ProjectID) sebelum
	// operasi lain (mis. delete/update kalau nanti ditambah).
	GetByID(ctx context.Context, id string) (*AlertRule, error)

	// ListByProjectID dipakai buat GET /projects/:projectId/alert-rules.
	ListByProjectID(ctx context.Context, projectID string) ([]*AlertRule, error)

	// ListActiveNewIssueRules dipakai event-driven handler di
	// ingest_consumer.go: ambil semua rule condition_type="new_issue"
	// MILIK satu project (project dari issue yang baru saja ter-grup).
	ListActiveNewIssueRules(ctx context.Context, projectID string) ([]*AlertRule, error)

	// ListActiveThresholdRules dipakai periodic ticker: ambil SEMUA rule
	// condition_type="threshold" LINTAS project sekaligus (ticker jalan
	// sekali per interval, bukan per-project).
	ListActiveThresholdRules(ctx context.Context) ([]*AlertRule, error)

	// Update meng-update field yang bisa diubah lewat PATCH. Usecase
	// bertanggung jawab ownership check (GetByID) + merge partial
	// request ke *AlertRule existing sebelum manggil ini — pola sama
	// seperti MonitorRepository.Update. Repository tidak cek row
	// exists (RowsAffected) karena usecase sudah pastikan lewat GetByID
	// sesaat sebelumnya.
	Update(ctx context.Context, r *AlertRule) error

	// Delete menghapus alert rule. Cascade ke alert_logs terkait
	// (ON DELETE CASCADE, lihat 03-DATABASE-DESIGN.md). Usecase
	// bertanggung jawab ownership check sebelum manggil ini.
	Delete(ctx context.Context, id string) error
}

// AlertLogRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres.
type AlertLogRepository interface {
	Create(ctx context.Context, l *AlertLog) error

	GetLastSentAt(ctx context.Context, alertRuleID, issueID string) (*time.Time, error)
}

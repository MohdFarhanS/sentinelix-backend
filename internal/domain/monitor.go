package domain

import (
	"context"
	"errors"
	"net/url"
	"time"
)

// Monitor status values. "unknown" dipakai sebelum check pertama jalan,
// diupdate jadi "up"/"down" oleh MonitorCheckerUsecase setiap selesai ping.
const (
	MonitorStatusUnknown 	= "unknown"
	MonitorStatusUp			= "up"
	MonitorStatusDown		= "down"
	MonitorSyncCreated = "monitor.created"
	MonitorSyncUpdated = "monitor.updated"
	MonitorSyncDeleted = "monitor.deleted"

	// DefaultFailureThreshold dipakai kalau request tidak mengisi
	// failure_threshold — sesuai FR-19.
	DefaultFailureThreshold = 3

	// MinIntervalSec sesuai FR-17: minimal 1 menit antar ping.
	MinIntervalSec = 60
)

// Monitor merepresentasikan satu URL yang dipantau berkala. Channel &
// ChannelTarget MILIK monitor ini sendiri — SENGAJA tidak lewat
// alert_rules (lihat 05-ARCHITECTURE.md), karena semantik "down" beda
// dari alert_rules (bukan windowed count, bukan threshold count, tapi N
// kegagalan BERTURUT-TURUT — lihat MonitorCheckerUsecase di
// check_monitor.go).

type Monitor struct {
	ID					string
	ProjectID			string
	URL					string
	IntervalSec			int
	Channel				string
	ChannelTarget		string
	FailureThreshold	int
	Status				string
	CreatedAt			time.Time
}

var (
	ErrMonitorNotFound					= errors.New("monitor not found")
	ErrMonitorURLRequired				= errors.New("url is required")
	ErrMonitorURLInvalid 				= errors.New("url must be a valid absolute http/https URL")
	ErrMonitorIntervalInvalid			= errors.New("interval_sec must be >= 60")
	ErrMonitorChannelInvalid			= errors.New("channel must be 'email' or 'slack'")
	ErrMonitorChannelTargetRequired		= errors.New("channel_target is required")
	ErrMonitorFailureThresholdInvalid	= errors.New("failure_threshold must be > 0")
)

// Validate menjaga integritas data sebelum masuk repository. Dipanggil
// SETELAH usecase mengisi default (FailureThreshold, IntervalSec) — lihat
// MonitorUsecase.Create di monitor.go, pola sama seperti
// AlertRule.Validate().
func (m *Monitor) Validate() error {
	if m.URL == "" {
		return ErrMonitorURLRequired
	}
	if err := validateMonitorURL(m.URL); err != nil {
		return err
	}
	if m.IntervalSec < MinIntervalSec {
		return ErrMonitorIntervalInvalid
	}
	if m.Channel != ChannelEmail && m.Channel != ChannelSlack {
		return ErrMonitorChannelInvalid
	}
	if m.ChannelTarget == "" {
		return ErrMonitorChannelTargetRequired
	}
	if m.FailureThreshold <= 0 {
		return ErrMonitorFailureThresholdInvalid
	}
	return nil
}

func validateMonitorURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return ErrMonitorURLInvalid
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ErrMonitorURLInvalid
	}
	return nil
}

// MonitorCheck adalah satu hasil ping. Disimpan ke tabel partitioned
// monitor_checks (lihat 03-DATABASE-DESIGN.md) — sama seperti events,
// bakal di-drop per partisi setelah 30 hari (retention policy).
type MonitorCheck struct {
	ID			string
	MonitorID	string
	StatusCode	int
	LatencyMs	int
	IsUp		bool
	CheckedAt	time.Time
}

// MonitorRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres.
type MonitorRepository interface {
	Create(ctx context.Context, m *Monitor) error
	GetByID(ctx context.Context, id string) (*Monitor, error)
	ListByProjectID(ctx context.Context, projectID string) ([]*Monitor, error)

	// ListAll dipakai worker startup — supervisor perlu rebuild semua
	// goroutine checker dari state DB saat proses baru nyala (lihat
	// prinsip kerja: worker restart harus rebuild, bukan asumsi state
	// lama masih ada).
	ListAll(ctx context.Context) ([]*Monitor, error)

	// Update meng-update SEMUA field yang bisa diubah lewat PATCH
	// (URL, IntervalSec, Channel, ChannelTarget, FailureThreshold).
	// Usecase yang tanggung jawab merge partial request ke *Monitor
	// existing sebelum manggil ini (pola sama seperti IssueUsecase).
	Update(ctx context.Context, m *Monitor) error

	// UpdateStatus dipanggil checker SETIAP SELESAI PING (bukan cuma
	// saat status berubah) — supaya kolom status.monitors selalu
	// reflect hasil check terakhir tanpa perlu query monitor_checks
	// tiap kali GET /monitors/:id dipanggil dashboard.
	UpdateStatus(ctx context.Context, id, status string) error

	Delete(ctx context.Context, id string) error
}


// MonitorCheckRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres.
type MonitorCheckRepository interface {
	Create(ctx context.Context, c *MonitorCheck) error

	// ListRecentByMonitorID dipakai MonitorCheckerUsecase buat evaluasi
	// consecutive failure (FR-19) — ambil N check TERAKHIR (ORDER BY
	// checked_at DESC LIMIT n), cek semua IsUp=false. Keputusan sadar:
	// on-the-fly query, BUKAN counter kolom terpisah — single source of
	// truth di monitor_checks, konsisten dengan alasan skip dedup_cache
	// Sprint 3 (atomic query cukup, tidak perlu state duplikat).
	ListRecentByMonitorID(ctx context.Context, monitorID string, limit int) ([]*MonitorCheck, error)

	// ListByMonitorID dipakai GET /monitors/:id/checks (query from/to
	// buat chart uptime dashboard). from/to nil berarti tanpa filter.
	ListByMonitorID(ctx context.Context, monitorID string, from, to *time.Time) ([]*MonitorCheck, error)
}
package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// --- MonitorRepository ---

type MonitorRepository struct {
	db *pgxpool.Pool
}

func NewMonitorRepository(db *pgxpool.Pool) *MonitorRepository {
	return &MonitorRepository{db: db}
}

func (r *MonitorRepository) Create(ctx context.Context, m *domain.Monitor) error {
	query := `
		INSERT INTO monitors (project_id, url, name, interval_sec, channel, channel_target, failure_threshold)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, status, created_at
	`

	return r.db.QueryRow(ctx, query,
		m.ProjectID, m.URL, m.Name, m.IntervalSec, m.Channel, m.ChannelTarget, m.FailureThreshold,
	).Scan(&m.ID, &m.Status, &m.CreatedAt)
}

func (r *MonitorRepository) GetByID(ctx context.Context, id string) (*domain.Monitor, error) {
	query := `
		SELECT id, project_id, url, name, interval_sec, channel, channel_target, failure_threshold, status, created_at
		FROM monitors
		WHERE id = $1
	`

	var m domain.Monitor
	var name *string
	err := r.db.QueryRow(ctx, query, id).Scan(
		&m.ID, &m.ProjectID, &m.URL, &name, &m.IntervalSec, &m.Channel,
		&m.ChannelTarget, &m.FailureThreshold, &m.Status, &m.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrMonitorNotFound
	}
	if err != nil {
		return nil, err
	}
	if name != nil {
		m.Name = *name
	}
	return &m, nil
}

func (r *MonitorRepository) ListByProjectID(ctx context.Context, projectID string) ([]*domain.Monitor, error) {
	return r.queryMonitors(ctx, `
		SELECT id, project_id, url, name, interval_sec, channel, channel_target, failure_threshold, status, created_at
		FROM monitors
		WHERE project_id = $1
		ORDER BY created_at DESC
	`, projectID)
}

// ListAll dipakai worker startup — supervisor rebuild semua goroutine
// checker dari state DB, lintas project sekaligus (sama pola dengan
// AlertRuleRepository.ListActiveThresholdRules di Sprint 6).
func (r *MonitorRepository) ListAll(ctx context.Context) ([]*domain.Monitor, error) {
	return r.queryMonitors(ctx, `
		SELECT id, project_id, url, name, interval_sec, channel, channel_target, failure_threshold, status, created_at
		FROM monitors
	`)
}

// queryMonitors helper kecil biar ListByProjectID & ListAll tidak
// duplikasi loop scan — pola sama seperti queryRules di alert_repo.go.
// name di-scan lewat *string dulu karena kolomnya nullable di DB.
func (r *MonitorRepository) queryMonitors(ctx context.Context, query string, args ...any) ([]*domain.Monitor, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monitors := []*domain.Monitor{}
	for rows.Next() {
		var m domain.Monitor
		var name *string
		if err := rows.Scan(
			&m.ID, &m.ProjectID, &m.URL, &name, &m.IntervalSec, &m.Channel,
			&m.ChannelTarget, &m.FailureThreshold, &m.Status, &m.CreatedAt,
		); err != nil {
			return nil, err
		}
		if name != nil {
			m.Name = *name
		}
		monitors = append(monitors, &m)
	}
	return monitors, rows.Err()
}

// Update meng-update field yang bisa diubah lewat PATCH. Usecase yang
// tanggung jawab merge partial request ke *Monitor existing sebelum
// manggil ini (baca GetByID dulu, override field yang dikirim, panggil
// Update) — repository tidak tahu apa yang "partial", cuma nerima
// state final yang mau ditulis.
func (r *MonitorRepository) Update(ctx context.Context, m *domain.Monitor) error {
	query := `
		UPDATE monitors
		SET url = $2, name = $3, interval_sec = $4, channel = $5, channel_target = $6, failure_threshold = $7
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query, m.ID, m.URL, m.Name, m.IntervalSec, m.Channel, m.ChannelTarget, m.FailureThreshold)
	return err
}

// UpdateStatus dipanggil checker setiap selesai ping (bukan Update biasa)
// — dipisah biar checker tidak perlu load & re-marshal seluruh struct
// Monitor cuma buat ubah 1 kolom tiap ping.
func (r *MonitorRepository) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE monitors SET status = $2 WHERE id = $1`, id, status)
	return err
}

func (r *MonitorRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM monitors WHERE id = $1`, id)
	return err
}

// --- MonitorCheckRepository ---

type MonitorCheckRepository struct {
	db *pgxpool.Pool
}

func NewMonitorCheckRepository(db *pgxpool.Pool) *MonitorCheckRepository {
	return &MonitorCheckRepository{db: db}
}

func (r *MonitorCheckRepository) Create(ctx context.Context, c *domain.MonitorCheck) error {
	query := `
		INSERT INTO monitor_checks (monitor_id, status_code, latency_ms, is_up)
		VALUES ($1, $2, $3, $4)
		RETURNING id, checked_at
	`
	return r.db.QueryRow(ctx, query, c.MonitorID, c.StatusCode, c.LatencyMs, c.IsUp).Scan(&c.ID, &c.CheckedAt)
}

// ListRecentByMonitorID dipakai MonitorCheckerUsecase buat evaluasi
// consecutive failure (FR-19) — ambil `limit` check TERAKHIR, caller yang
// cek semua IsUp=false. On-the-fly query, konsisten dengan keputusan
// arsitektur (bukan counter kolom terpisah).
func (r *MonitorCheckRepository) ListRecentByMonitorID(ctx context.Context, monitorID string, limit int) ([]*domain.MonitorCheck, error) {
	query := `
		SELECT id, monitor_id, status_code, latency_ms, is_up, checked_at
		FROM monitor_checks
		WHERE monitor_id = $1
		ORDER BY checked_at DESC
		LIMIT $2
	`
	rows, err := r.db.Query(ctx, query, monitorID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checks := []*domain.MonitorCheck{}
	for rows.Next() {
		var c domain.MonitorCheck
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.StatusCode, &c.LatencyMs, &c.IsUp, &c.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, &c)
	}
	return checks, rows.Err()
}

// ListByMonitorID dipakai GET /monitors/:id/checks (chart uptime
// dashboard). from/to nil berarti tanpa filter — dicek lewat
// "$N::timestamptz IS NULL OR ..." di SQL, bukan dynamic query building,
// biar tidak perlu branching string query di Go (pgx otomatis kirim NULL
// kalau arg-nya *time.Time bernilai nil).
func (r *MonitorCheckRepository) ListByMonitorID(ctx context.Context, monitorID string, from, to *time.Time) ([]*domain.MonitorCheck, error) {
	query := `
		SELECT id, monitor_id, status_code, latency_ms, is_up, checked_at
		FROM monitor_checks
		WHERE monitor_id = $1
		  AND ($2::timestamptz IS NULL OR checked_at >= $2)
		  AND ($3::timestamptz IS NULL OR checked_at <= $3)
		ORDER BY checked_at DESC
	`
	rows, err := r.db.Query(ctx, query, monitorID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checks := []*domain.MonitorCheck{}
	for rows.Next() {
		var c domain.MonitorCheck
		if err := rows.Scan(&c.ID, &c.MonitorID, &c.StatusCode, &c.LatencyMs, &c.IsUp, &c.CheckedAt); err != nil {
			return nil, err
		}
		checks = append(checks, &c)
	}
	return checks, rows.Err()
}
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// StatusRepository — READ-ONLY, dipakai SEMATA-MATA oleh cmd/status-api
// (lihat 05-ARCHITECTURE.md §6c). Cuma import domain (tipe balik) dan
// pgx (driver DB) — tidak ada dependency ke usecase/handler dashboard.
type StatusRepository struct {
	db *pgxpool.Pool
}

func NewStatusRepository(db *pgxpool.Pool) *StatusRepository {
	return &StatusRepository{db: db}
}

// GetProjectStatusBySlug mengambil project + semua monitor + uptime_30d
// dalam 2 query (bukan 1 query window-function raksasa) — lebih gampang
// dibaca & di-maintain; traffic status page tidak cukup tinggi buat 2
// round-trip jadi masalah performa (apalagi di-ISR cache di frontend).
func (r *StatusRepository) GetProjectStatusBySlug(ctx context.Context, slug string) (*domain.ProjectStatusData, error) {
	var projectID, projectName string
	err := r.db.QueryRow(ctx, `
		SELECT id, name FROM projects WHERE slug = $1
	`, slug).Scan(&projectID, &projectName)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// uptime_30d dihitung on-the-fly, dibatasi 30 hari via WHERE clause
	// di JOIN (bukan pre-agregasi) — sesuai keputusan sebelumnya: partisi
	// monitor_checks sudah otomatis di-drop setelah 30 hari
	// (03-DATABASE-DESIGN.md §5), jadi data yang di-scan sudah dibatasi
	// retention policy, tidak perlu counter/agregasi terpisah (YAGNI).
	//
	// Monitor TANPA check sama sekali (baru dibuat) -> COUNT(mc.id)=0 ->
	// NULLIF cegah div-by-zero -> COALESCE fallback ke 100 (dianggap
	// "belum ada bukti masalah", konsisten dengan perlakuan status
	// "unknown" di ComputeOverallStatus).
	rows, err := r.db.Query(ctx, `
		SELECT
			m.name,
			m.url,
			m.status,
			COALESCE(
				ROUND(
					(COUNT(mc.id) FILTER (WHERE mc.is_up = true))::numeric
					/ NULLIF(COUNT(mc.id), 0) * 100,
					2
				),
				100
			) AS uptime_30d
		FROM monitors m
		LEFT JOIN monitor_checks mc
			ON mc.monitor_id = m.id
			AND mc.checked_at >= now() - interval '30 days'
		WHERE m.project_id = $1
		GROUP BY m.id, m.name, m.url, m.status
		ORDER BY m.created_at ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	monitors := []domain.MonitorStatusData{}
	for rows.Next() {
		var name, url, status string
		var uptime30d float64
		if err := rows.Scan(&name, &url, &status, &uptime30d); err != nil {
			return nil, err
		}
		// Fallback name->url dilakukan di sini (bukan reuse
		// Monitor.DisplayName()) karena repo ini tidak query full struct
		// Monitor, cuma kolom yang perlu buat status page.
		displayName := name
		if displayName == "" {
			displayName = url
		}
		monitors = append(monitors, domain.MonitorStatusData{
			Name:      displayName,
			RawStatus: status,
			Uptime30d: uptime30d,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &domain.ProjectStatusData{
		ProjectName: projectName,
		Monitors:    monitors,
	}, nil
}

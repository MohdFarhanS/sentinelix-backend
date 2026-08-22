package postgres

import (
	"context"
	"time"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type IssueRepository struct {
	db *pgxpool.Pool
}

func NewIssueRepository(db *pgxpool.Pool) *IssueRepository {
	return &IssueRepository{db: db}
}

// Upsert pakai INSERT ... ON CONFLICT (project_id, fingerprint) DO UPDATE,
// memanfaatkan UNIQUE(project_id, fingerprint) di 03-DATABASE-DESIGN.md.
// Ini satu round-trip atomik ke Postgres, jadi tidak butuh SELECT-dulu-baru-
// INSERT (yang rawan race condition kalau 2 worker proses fingerprint sama
// di waktu bersamaan).
//
// Trik `(xmax = 0)`: kolom system Postgres `xmax` bernilai 0 pada baris yang
// baru saja di-INSERT, dan berisi id transaksi lain kalau baris itu hasil
// UPDATE (lewat jalur ON CONFLICT). Jadi ini cara murah buat tahu apakah
// baris ini baru dibuat atau update dari baris lama, tanpa query tambahan.
func (r *IssueRepository) Upsert(
	ctx context.Context,
	projectID, fingerprint, title, level string,
	occurredAt time.Time,
) (*domain.Issue, bool, error) {
	query := `
		INSERT INTO issues (project_id, fingerprint, title, level, first_seen, last_seen, count)
		VALUES ($1, $2, $3, $4, $5, $5, 1)
		ON CONFLICT (project_id, fingerprint) DO UPDATE
		SET count = issues.count + 1,
		    last_seen = $5
		RETURNING id, project_id, fingerprint, title, level, status, first_seen, last_seen, count, (xmax = 0) AS was_created
	`

	var issue domain.Issue
	var wasCreated bool
	err := r.db.QueryRow(ctx, query, projectID, fingerprint, title, level, occurredAt).Scan(
		&issue.ID, &issue.ProjectID, &issue.Fingerprint, &issue.Title, &issue.Level,
		&issue.Status, &issue.FirstSeen, &issue.LastSeen, &issue.Count, &wasCreated,
	)
	if err != nil {
		return nil, false, err
	}
	return &issue, wasCreated, nil
}

func (r *IssueRepository) List(ctx context.Context, filter domain.IssueFilter) (*domain.IssueListResult, error) {
	query := `
		SELECT id, project_id, fingerprint, title, level, status,
		       first_seen, last_seen, count,
		       COUNT(*) OVER() AS total
		FROM issues
		WHERE project_id = $1
		  AND ($2 = '' OR status = $2)
		ORDER BY last_seen DESC
		LIMIT $3 OFFSET $4
	`
	offset := (filter.Page - 1) * filter.Limit

	rows, err := r.db.Query(ctx, query, filter.ProjectID, filter.Status, filter.Limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := &domain.IssueListResult{Issues: []*domain.Issue{}}
	for rows.Next() {
		var issue domain.Issue
		if err := rows.Scan(
			&issue.ID, &issue.ProjectID, &issue.Fingerprint, &issue.Title, &issue.Level,
			&issue.Status, &issue.FirstSeen, &issue.LastSeen, &issue.Count, &result.Total,
		); err != nil {
			return nil, err
		}
		result.Issues = append(result.Issues, &issue)
	}
	return result, rows.Err()
}

func (r *IssueRepository) GetByID(ctx context.Context, id string) (*domain.Issue, error) {
	query := `
		SELECT id, project_id, fingerprint, title, level, status,
			first_seen, last_seen, count
		FROM issues
		WHERE id = $1
	`

	var issue domain.Issue
	err := r.db.QueryRow(ctx, query, id).Scan(
		&issue.ID, &issue.ProjectID, &issue.Fingerprint, &issue.Title, &issue.Level,
		&issue.Status, &issue.FirstSeen, &issue.LastSeen, &issue.Count,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrIssueNotFound
		}
		return nil, err
	}
	return &issue, nil
}
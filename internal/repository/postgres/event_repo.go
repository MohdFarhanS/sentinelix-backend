package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type EventRepository struct {
	db *pgxpool.Pool
}

func NewEventRepository(db *pgxpool.Pool) *EventRepository {
	return &EventRepository{db: db}
}

// Insert menyimpan event mentah ke tabel `events` (partitioned by month,
// lihat 03-DATABASE-DESIGN.md §3). event.Context (map[string]any) di-encode
// jadi JSON dan disimpan di kolom `payload` (jsonb).
func (r *EventRepository) Insert(ctx context.Context, event *domain.Event) error {
	payload, err := json.Marshal(event.Context)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO events (issue_id, project_id, payload, stack_trace, occurred_at)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = r.db.Exec(ctx, query, event.IssueID, event.ProjectID, payload, event.StackTrace, event.OccurredAt)
	return err
}

// ListByIssueID pakai index idx_events_issue(issue_id, occurred_at DESC)
// yang sudah ada di 03-DATABASE-DESIGN.md, jadi query ini tidak perlu
// sequential scan walau tabel 'events' partitioned & besar.
func (r *EventRepository) ListByIssueID(ctx context.Context, issueID string, limit int) ([]*domain.Event, error) {
	query := `
		SELECT id, issue_id, project_id, payload, stack_trace, occurred_at
		FROM events
		WHERE issue_id = $1
		ORDER BY occurred_at DESC
		LIMIT $2
	`

	rows, err := r.db.Query(ctx, query, issueID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := []*domain.Event{}
	for rows.Next() {
		var event domain.Event
		var payload []byte

		if err := rows.Scan(
			&event.ID, &event.IssueID, &event.ProjectID,
			&payload, &event.StackTrace, &event.OccurredAt,
		); err != nil {
			return nil, err
		}

		if len(payload) > 0 {
			if err := json.Unmarshal(payload, &event.Context); err != nil {
				return nil, err
			}
		}

		events = append(events, &event)
	}
	return events, rows.Err()
}

// CountGroupedByIssueSince satu query GROUP BY untuk semua issue dalam
// satu project sekaligus (bukan N+1 per issue) — dipakai worker threshold
// ticker (Sprint 6). Butuh idx_events_project_occurred(project_id,
// occurred_at DESC) biar tidak sequential scan (lihat catatan migration).
func (r *EventRepository) CountGroupedByIssueSince(ctx context.Context, projectID string, since time.Time) (map[string]int, error) {
	query := `
		SELECT issue_id, COUNT(*)
		FROM events
		WHERE project_id = $1 AND occurred_at > $2
		GROUP BY issue_id
	`

	rows, err := r.db.Query(ctx, query, projectID, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var issueID string
		var count int
		if err := rows.Scan(&issueID, &count); err != nil {
			return nil, err
		}
		counts[issueID] = count
	}
	return counts, rows.Err()
}
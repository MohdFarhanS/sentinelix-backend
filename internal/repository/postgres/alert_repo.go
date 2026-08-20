package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// --- AlertRuleRepository ---

type AlertRuleRepository struct {
	db *pgxpool.Pool
}

func NewAlertRuleRepository(db *pgxpool.Pool) *AlertRuleRepository {
	return &AlertRuleRepository{db: db}
}

func (r *AlertRuleRepository) Create(ctx context.Context, rule *domain.AlertRule) error {
	query := `
		INSERT INTO alert_rules (project_id, condition_type, threshold, window_minutes, cooldown_minutes, channel, channel_target)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at
	`
	
	return r.db.QueryRow(ctx, query, 
		rule.ProjectID, rule.ConditionType, rule.Threshold, rule.WindowMinutes, 
		rule.CooldownMinutes, rule.Channel, rule.ChannelTarget,
	).Scan(&rule.ID, &rule.CreatedAt)
}

func (r *AlertRuleRepository) GetByID(ctx context.Context, id string) (*domain.AlertRule, error) {
	query := `
		SELECT id, project_id, condition_type, threshold, window_minutes, cooldown_minutes, channel, channel_target, created_at
		FROM alert_rules
		WHERE id = $1
	`

	var rule domain.AlertRule
	err := r.db.QueryRow(ctx, query, id).Scan(
		&rule.ID, &rule.ProjectID, &rule.ConditionType, &rule.Threshold,
		&rule.WindowMinutes, &rule.CooldownMinutes, &rule.Channel, &rule.ChannelTarget, &rule.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrAlertRuleNotFound
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

func (r *AlertRuleRepository) ListByProjectID(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	return r.queryRules(ctx, `
		SELECT id, project_id, condition_type, threshold, window_minutes, cooldown_minutes, channel, channel_target, created_at
		FROM alert_rules
		WHERE project_id = $1
	`, projectID)
}

func (r *AlertRuleRepository) ListActiveNewIssueRules(ctx context.Context, projectID string) ([]*domain.AlertRule, error) {
	return r.queryRules(ctx, `
		SELECT id, project_id, condition_type, threshold, window_minutes, cooldown_minutes, channel, channel_target, created_at
		FROM alert_rules
		WHERE project_id = $1 AND condition_type = 'new_issue'
	`, projectID)
}

func (r *AlertRuleRepository) ListActiveThresholdRules(ctx context.Context) ([]*domain.AlertRule, error) {
	return r.queryRules(ctx, `
		SELECT id, project_id, condition_type, threshold, window_minutes, cooldown_minutes, channel, channel_target, created_at
		FROM alert_rules
		WHERE condition_type = 'threshold'
	`)
}

// queryRules helper kecil biar 3 method List* di atas tidak duplikasi
// loop scan yang sama persis — cuma query string & args yang beda.
func (r *AlertRuleRepository) queryRules(ctx context.Context, query string, args ...any) ([]*domain.AlertRule, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	rules := []*domain.AlertRule{}
	for rows.Next() {
		var rule domain.AlertRule
		if err := rows.Scan(
			&rule.ID, &rule.ProjectID, &rule.ConditionType, &rule.Threshold,
			&rule.WindowMinutes, &rule.CooldownMinutes, &rule.Channel, &rule.ChannelTarget, &rule.CreatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, &rule)
	}
	return rules, rows.Err()
}

func (r *AlertRuleRepository) Update(ctx context.Context, rule *domain.AlertRule) error {
	query := `
		UPDATE alert_rules
		SET condition_type = $2, threshold = $3, window_minutes = $4,
		    cooldown_minutes = $5, channel = $6, channel_target = $7
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query,
		rule.ID, rule.ConditionType, rule.Threshold, rule.WindowMinutes,
		rule.CooldownMinutes, rule.Channel, rule.ChannelTarget,
	)
	return err
}

func (r *AlertRuleRepository) Delete(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1`, id)
	return err
}

// --- AlertLogRepository ---

type AlertLogRepository struct {
	db *pgxpool.Pool
}

func NewAlertLogRepository(db *pgxpool.Pool) *AlertLogRepository {
	return &AlertLogRepository{db: db}
}

func (r *AlertLogRepository) Create(ctx context.Context, log *domain.AlertLog) error {
	query := `
		INSERT INTO alert_logs (alert_rule_id, issue_id)
		VALUES ($1, $2)
		RETURNING id, sent_at
	`
	return r.db.QueryRow(ctx, query, log.AlertRuleID, log.IssueID).Scan(&log.ID, &log.SentAt)
}

// GetLastSentAt return (nil, nil) kalau belum pernah ada log — itu kondisi
// normal (belum pernah kirim alert utk kombinasi rule+issue ini), BUKAN
// error. Caller (evaluate_alert.go) yang decide apa artinya nil ini.
func (r *AlertLogRepository) GetLastSentAt(ctx context.Context, alertRuleID, issueID string) (*time.Time, error) {
	query := `
		SELECT sent_at
		FROM alert_logs
		WHERE alert_rule_id = $1 AND issue_id = $2
		ORDER BY sent_at DESC
		LIMIT 1
	`
	var sentAt time.Time
	err := r.db.QueryRow(ctx, query, alertRuleID, issueID).Scan(&sentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sentAt, nil
}
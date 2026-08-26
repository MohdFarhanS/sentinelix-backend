package postgres_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	"github.com/MohdFarhanS/sentinelix-backend/internal/testutil"
)

func TestProjectRepository_Delete_Success(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewProjectRepository(pool)
	user := createTestUser(t, pool)

	project := &domain.Project{
		UserID:     user.ID,
		Name:       "To Be Deleted",
		Slug:       "to-be-deleted-abc123",
		APIKeyHash: "irrelevant-hash-for-this-test",
	}
	require.NoError(t, repo.Create(context.Background(), project))

	err := repo.Delete(context.Background(), project.ID)
	require.NoError(t, err)

	_, err = repo.GetByID(context.Background(), project.ID)
	assert.ErrorIs(t, err, domain.ErrProjectNotFound, "project harus benar-benar hilang setelah Delete")
}

func TestProjectRepository_Delete_CascadesToIssues(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewProjectRepository(pool)
	user := createTestUser(t, pool)

	project := &domain.Project{
		UserID:     user.ID,
		Name:       "Project With Issues",
		Slug:       "project-with-issues-abc123",
		APIKeyHash: "irrelevant-hash",
	}
	require.NoError(t, repo.Create(context.Background(), project))

	_, err := pool.Exec(context.Background(), `
		INSERT INTO issues (project_id, fingerprint, title)
		VALUES ($1, 'fp-1', 'Some Error')
	`, project.ID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(context.Background(), project.ID))

	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM issues WHERE project_id = $1`, project.ID,
	).Scan(&count))
	assert.Equal(t, 0, count, "issue harus ikut terhapus (CASCADE) setelah project dihapus")
}

// TestProjectRepository_Delete_CascadesToAlertRules — alert_rules.project_id
// ON DELETE CASCADE (000003_create_alert_tables.up.sql). alert_logs juga
// ikut ke-cascade SECARA TIDAK LANGSUNG lewat alert_rules (alert_logs.
// alert_rule_id ON DELETE CASCADE) — dua-duanya diverifikasi di sini
// sekalian, bukan file terpisah, karena hubungannya rantai 1 arah yang
// sama (project -> alert_rules -> alert_logs).
func TestProjectRepository_Delete_CascadesToAlertRules(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewProjectRepository(pool)
	user := createTestUser(t, pool)

	project := &domain.Project{
		UserID:     user.ID,
		Name:       "Project With Alert Rules",
		Slug:       "project-with-alert-rules-abc123",
		APIKeyHash: "irrelevant-hash",
	}
	require.NoError(t, repo.Create(context.Background(), project))

	var alertRuleID string
	err := pool.QueryRow(context.Background(), `
		INSERT INTO alert_rules (project_id, condition_type, channel, channel_target)
		VALUES ($1, 'new_issue', 'slack', 'https://hooks.slack.com/test')
		RETURNING id
	`, project.ID).Scan(&alertRuleID)
	require.NoError(t, err)

	_, err = pool.Exec(context.Background(), `
		INSERT INTO alert_logs (alert_rule_id, issue_id) VALUES ($1, gen_random_uuid())
	`, alertRuleID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(context.Background(), project.ID))

	var alertRuleCount, alertLogCount int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM alert_rules WHERE project_id = $1`, project.ID,
	).Scan(&alertRuleCount))
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM alert_logs WHERE alert_rule_id = $1`, alertRuleID,
	).Scan(&alertLogCount))

	assert.Equal(t, 0, alertRuleCount, "alert_rules harus ikut terhapus (CASCADE)")
	assert.Equal(t, 0, alertLogCount, "alert_logs harus ikut terhapus (CASCADE via alert_rules, dua tingkat)")
}

// TestProjectRepository_Delete_CascadesToMonitors — monitors.project_id
// ON DELETE CASCADE (03-DATABASE-DESIGN.md §3).
func TestProjectRepository_Delete_CascadesToMonitors(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewProjectRepository(pool)
	user := createTestUser(t, pool)

	project := &domain.Project{
		UserID:     user.ID,
		Name:       "Project With Monitors",
		Slug:       "project-with-monitors-abc123",
		APIKeyHash: "irrelevant-hash",
	}
	require.NoError(t, repo.Create(context.Background(), project))

	_, err := pool.Exec(context.Background(), `
		INSERT INTO monitors (project_id, url, channel, channel_target)
		VALUES ($1, 'https://example.com/health', 'email', 'ops@sentinelix.com')
	`, project.ID)
	require.NoError(t, err)

	require.NoError(t, repo.Delete(context.Background(), project.ID))

	var count int
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM monitors WHERE project_id = $1`, project.ID,
	).Scan(&count))
	assert.Equal(t, 0, count, "monitor harus ikut terhapus (CASCADE) setelah project dihapus")
}

func TestProjectRepository_Delete_NonExistent_NoError(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewProjectRepository(pool)

	err := repo.Delete(context.Background(), "00000000-0000-0000-0000-000000000000")
	assert.NoError(t, err)
}
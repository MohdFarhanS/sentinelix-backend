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

func TestAuditLogRepository_Create(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewAuditLogRepository(pool)
	user := createTestUser(t, pool)

	resourceID := "11111111-1111-1111-1111-111111111111"
	log := &domain.AuditLog{
		ActorUserID:  &user.ID,
		Action:       domain.ActionProjectAPIKeyCreated,
		ResourceType: domain.ResourceTypeProject,
		ResourceID:   resourceID,
		Metadata:     map[string]any{"project_name": "Test Project"},
	}

	err := repo.Create(context.Background(), log)
	require.NoError(t, err)
	assert.NotEmpty(t, log.ID)
	assert.False(t, log.CreatedAt.IsZero())

	// Verifikasi langsung ke DB — pastikan metadata JSONB tersimpan &
	// terbaca balik dengan benar, bukan cuma "insert tidak error".
	var storedAction, storedResourceType, storedMetadata string
	err = pool.QueryRow(context.Background(),
		`SELECT action, resource_type, metadata::text FROM audit_logs WHERE id = $1`, log.ID,
	).Scan(&storedAction, &storedResourceType, &storedMetadata)
	require.NoError(t, err)
	assert.Equal(t, domain.ActionProjectAPIKeyCreated, storedAction)
	assert.Equal(t, domain.ResourceTypeProject, storedResourceType)
	assert.Contains(t, storedMetadata, "Test Project")
}

func TestAuditLogRepository_Create_NilMetadata(t *testing.T) {
	// Metadata nil HARUS bisa tersimpan (kolom JSONB nullable) — bukan
	// semua action perlu detail tambahan.
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewAuditLogRepository(pool)
	user := createTestUser(t, pool)

	log := &domain.AuditLog{
		ActorUserID:  &user.ID,
		Action:       domain.ActionAlertRuleDeleted,
		ResourceType: domain.ResourceTypeAlertRule,
		ResourceID:   "22222222-2222-2222-2222-222222222222",
		Metadata:     nil,
	}

	err := repo.Create(context.Background(), log)
	assert.NoError(t, err)
}

// TestAuditLogRepository_ActorDeleted_LogSurvives — test PALING penting
// di file ini. Bukti langsung dari keputusan arsitektur ON DELETE SET
// NULL (bukan CASCADE): baris audit_logs HARUS tetap ada setelah user
// dihapus, cuma actor_user_id-nya jadi NULL — bukan soal "Create() sukses
// insert", tapi soal keputusan desain paling krusial audit log benar
// diimplementasikan di skema.
func TestAuditLogRepository_ActorDeleted_LogSurvives(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewAuditLogRepository(pool)
	user := createTestUser(t, pool)

	log := &domain.AuditLog{
		ActorUserID:  &user.ID,
		Action:       domain.ActionProjectDeleted,
		ResourceType: domain.ResourceTypeProject,
		ResourceID:   "33333333-3333-3333-3333-333333333333",
		Metadata:     map[string]any{"project_name": "Soon Orphaned"},
	}
	require.NoError(t, repo.Create(context.Background(), log))

	_, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	require.NoError(t, err)

	var actorUserID *string
	var action string
	err = pool.QueryRow(context.Background(),
		`SELECT actor_user_id, action FROM audit_logs WHERE id = $1`, log.ID,
	).Scan(&actorUserID, &action)
	require.NoError(t, err, "baris audit_logs HARUS masih ada walau user-nya sudah dihapus")
	assert.Nil(t, actorUserID, "actor_user_id harus jadi NULL (ON DELETE SET NULL), bukan ikut CASCADE")
	assert.Equal(t, domain.ActionProjectDeleted, action, "data lain di baris tetap utuh")
}
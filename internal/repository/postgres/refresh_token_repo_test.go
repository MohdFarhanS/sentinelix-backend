package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	"github.com/MohdFarhanS/sentinelix-backend/internal/testutil"
)

func TestRefreshTokenRepository_CreateAndFindByTokenHash(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)
	user := createTestUser(t, pool)

	token := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: "hash-abc123",
		ExpiresAt: time.Now().UTC().Add(30 * 24 * time.Hour),
	}

	err := repo.Create(context.Background(), token)
	require.NoError(t, err)
	assert.NotEmpty(t, token.ID, "ID harus ke-generate DB (gen_random_uuid())")
	assert.False(t, token.CreatedAt.IsZero(), "CreatedAt harus ke-generate DB (now())")

	found, err := repo.FindByTokenHash(context.Background(), "hash-abc123")
	require.NoError(t, err)
	assert.Equal(t, token.ID, found.ID)
	assert.Equal(t, user.ID, found.UserID)
	assert.Nil(t, found.RevokedAt, "token baru belum pernah di-revoke")
}

func TestRefreshTokenRepository_FindByTokenHash_NotFound(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)

	_, err := repo.FindByTokenHash(context.Background(), "hash-yang-tidak-pernah-ada")

	assert.ErrorIs(t, err, domain.ErrRefreshTokenNotFound)
}

func TestRefreshTokenRepository_Revoke(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)
	user := createTestUser(t, pool)

	token := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: "hash-to-revoke",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, repo.Create(context.Background(), token))

	err := repo.Revoke(context.Background(), token.ID)
	require.NoError(t, err)

	found, err := repo.FindByTokenHash(context.Background(), "hash-to-revoke")
	require.NoError(t, err)
	assert.NotNil(t, found.RevokedAt, "revoked_at harus terisi setelah Revoke()")
}

// TestRefreshTokenRepository_Revoke_Idempotent — query Revoke() pakai
// `WHERE revoked_at IS NULL`, jadi panggilan kedua cuma affect 0 baris,
// BUKAN error. Penting karena jalur normal (AuthUsecase.Refresh) dan
// reuse-detection (RevokeAllByUserID) bisa overlap kalau kejadian hampir
// bersamaan.
func TestRefreshTokenRepository_Revoke_Idempotent(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)
	user := createTestUser(t, pool)

	token := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: "hash-double-revoke",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, repo.Create(context.Background(), token))

	require.NoError(t, repo.Revoke(context.Background(), token.ID))
	err := repo.Revoke(context.Background(), token.ID)
	assert.NoError(t, err, "revoke kedua pada token yang sama tidak boleh error")
}

func TestRefreshTokenRepository_RevokeAllByUserID(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)
	user := createTestUser(t, pool)

	// User ini punya 2 token aktif (simulasi 2 device/browser berbeda).
	token1 := &domain.RefreshToken{UserID: user.ID, TokenHash: "hash-device-1", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	token2 := &domain.RefreshToken{UserID: user.ID, TokenHash: "hash-device-2", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	require.NoError(t, repo.Create(context.Background(), token1))
	require.NoError(t, repo.Create(context.Background(), token2))

	err := repo.RevokeAllByUserID(context.Background(), user.ID)
	require.NoError(t, err)

	found1, _ := repo.FindByTokenHash(context.Background(), "hash-device-1")
	found2, _ := repo.FindByTokenHash(context.Background(), "hash-device-2")
	assert.NotNil(t, found1.RevokedAt, "token device 1 harus ikut ke-revoke")
	assert.NotNil(t, found2.RevokedAt, "token device 2 harus ikut ke-revoke")
}

// TestRefreshTokenRepository_UserDeleted_TokenCascades — refresh_tokens.user_id
// pakai ON DELETE CASCADE (beda dari audit_logs.actor_user_id yang SET
// NULL) — hapus user harus ikut hapus SEMUA refresh token miliknya.
func TestRefreshTokenRepository_UserDeleted_TokenCascades(t *testing.T) {
	pool := testutil.NewPostgresPool(t)
	repo := postgres.NewRefreshTokenRepository(pool)
	user := createTestUser(t, pool)

	token := &domain.RefreshToken{
		UserID:    user.ID,
		TokenHash: "hash-cascade-test",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, repo.Create(context.Background(), token))

	_, err := pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, user.ID)
	require.NoError(t, err)

	_, err = repo.FindByTokenHash(context.Background(), "hash-cascade-test")
	assert.ErrorIs(t, err, domain.ErrRefreshTokenNotFound, "token harus ikut terhapus (CASCADE) setelah user dihapus")
}
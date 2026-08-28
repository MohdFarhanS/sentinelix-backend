package redis_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/testutil"
)

// fakeProjectRepo -- inner repository palsu, dipakai buat menghitung
// berapa kali GetByAPIKeyHash BENERAN nyampe ke "Postgres" (di sini
// simulasi doang, tidak ada Postgres asli). Ini yang membuktikan cache
// benar-benar mencegah panggilan berulang, bukan cuma asumsi.
type fakeProjectRepo struct {
	callCount int
	project   *domain.Project
	err       error
}

func (f *fakeProjectRepo) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.Project, error) {
	f.callCount++
	if f.err != nil {
		return nil, f.err
	}
	return f.project, nil
}

func (f *fakeProjectRepo) Create(ctx context.Context, p *domain.Project) error { return nil }

func (f *fakeProjectRepo) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	if f.project != nil && f.project.ID == id {
		return f.project, nil
	}
	return nil, domain.ErrProjectNotFound
}

func (f *fakeProjectRepo) ListByUserID(ctx context.Context, userID string) ([]*domain.Project, error) {
	return nil, nil
}

func (f *fakeProjectRepo) Delete(ctx context.Context, id string) error { return nil }

func TestCachedProjectRepository_CacheHit_AvoidsInnerCall(t *testing.T) {
	client := testutil.NewRedisClient(t)
	inner := &fakeProjectRepo{project: &domain.Project{ID: "project-1", APIKeyHash: "hash-1"}}
	repo := redis.NewCachedProjectRepository(inner, client)
	ctx := context.Background()

	_, err := repo.GetByAPIKeyHash(ctx, "hash-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, inner.callCount, "lookup pertama harus nyampe ke inner repo")

	_, err = repo.GetByAPIKeyHash(ctx, "hash-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, inner.callCount, "lookup kedua harus dari cache, TIDAK nambah callCount")
}

func TestCachedProjectRepository_DifferentKeys_BothHitInner(t *testing.T) {
	client := testutil.NewRedisClient(t)
	inner := &fakeProjectRepo{project: &domain.Project{ID: "project-1", APIKeyHash: "hash-1"}}
	repo := redis.NewCachedProjectRepository(inner, client)
	ctx := context.Background()

	_, _ = repo.GetByAPIKeyHash(ctx, "hash-1")
	_, _ = repo.GetByAPIKeyHash(ctx, "hash-2")

	assert.Equal(t, 2, inner.callCount, "2 API key hash berbeda harus 2x nyampe inner repo (cache key beda)")
}

func TestCachedProjectRepository_ErrorNeverCached(t *testing.T) {
	client := testutil.NewRedisClient(t)
	inner := &fakeProjectRepo{err: domain.ErrProjectNotFound}
	repo := redis.NewCachedProjectRepository(inner, client)
	ctx := context.Background()

	_, err1 := repo.GetByAPIKeyHash(ctx, "hash-invalid")
	_, err2 := repo.GetByAPIKeyHash(ctx, "hash-invalid")

	assert.ErrorIs(t, err1, domain.ErrProjectNotFound)
	assert.ErrorIs(t, err2, domain.ErrProjectNotFound)
	assert.Equal(t, 2, inner.callCount, "error TIDAK boleh di-cache -- tiap panggilan harus tetap coba inner repo lagi")
}

func TestCachedProjectRepository_Delete_InvalidatesCache(t *testing.T) {
	client := testutil.NewRedisClient(t)
	inner := &fakeProjectRepo{project: &domain.Project{ID: "project-1", APIKeyHash: "hash-1"}}
	repo := redis.NewCachedProjectRepository(inner, client)
	ctx := context.Background()

	_, _ = repo.GetByAPIKeyHash(ctx, "hash-1")
	assert.Equal(t, 1, inner.callCount)

	err := repo.Delete(ctx, "project-1")
	assert.NoError(t, err)

	_, _ = repo.GetByAPIKeyHash(ctx, "hash-1")
	assert.Equal(t, 2, inner.callCount, "setelah Delete, cache harus ke-invalidate -- lookup berikutnya wajib fresh dari inner repo")
}
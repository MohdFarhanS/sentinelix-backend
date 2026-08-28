package redis

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// projectCacheTTL — SENGAJA diselaraskan dengan threshold auto-suspend
// Neon (~5 menit), bukan angka acak. Efeknya dua arah: (1) mengurangi
// query Postgres pada jalur ingest yang paling sering dipanggil, bantu
// Neon benar-benar sempat idle & auto-suspend (hemat CU-hrs); (2) jadi
// fallback resiliensi — kalau Postgres/Neon lagi tidak bisa diakses
// (suspend/limit compute habis), project yang baru saja aktif ingest
// TETAP BISA lanjut tanpa query DB sama sekali, selama TTL ini belum
// kadaluarsa.
const projectCacheTTL = 5 * time.Minute

// CachedProjectRepository — decorator di atas domain.ProjectRepository,
// BUKAN implementasi baru dari nol. Cuma GetByAPIKeyHash yang di-cache
// (hot path ingest, dipanggil tiap POST /ingest/event); method lain
// (Create, GetByID, ListByUserID, Delete) diteruskan APA ADANYA ke
// repository asli — operasi CRUD project harus selalu baca data
// ter-terbaru, tidak boleh ikut ke-cache.
//
// Trade-off yang diterima: window staleness selama TTL (5 menit) — kalau
// project dihapus TANPA lewat Delete() di bawah (edge case yang seharusnya
// tidak terjadi lewat API normal), API key-nya masih bisa dipakai ingest
// sampai TTL habis. Ini konsisten dengan filosofi fail-open yang sudah
// dipakai di rate limiter (Sprint 10): lebih baik terima event yang
// "seharusnya" sudah tidak valid selama beberapa menit, daripada menolak
// event ASLI gara-gara ketersediaan Postgres bermasalah.
type CachedProjectRepository struct {
	inner  domain.ProjectRepository
	client *redis.Client
}

func NewCachedProjectRepository(inner domain.ProjectRepository, client *redis.Client) *CachedProjectRepository {
	return &CachedProjectRepository{inner: inner, client: client}
}

func cacheKeyForAPIKeyHash(apiKeyHash string) string {
	return "project_cache:apikey:" + apiKeyHash
}

func (r *CachedProjectRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.Project, error) {
	key := cacheKeyForAPIKeyHash(apiKeyHash)

	if cached, err := r.client.Get(ctx, key).Result(); err == nil {
		var project domain.Project
		if jsonErr := json.Unmarshal([]byte(cached), &project); jsonErr == nil {
			return &project, nil
		}
		// Cache corrupt/format lama -- treat sebagai cache miss, JANGAN
		// gagalkan request cuma gara-gara data cache rusak.
	}

	project, err := r.inner.GetByAPIKeyHash(ctx, apiKeyHash)
	if err != nil {
		// SENGAJA tidak cache hasil error (termasuk ErrProjectNotFound) --
		// scope cache ini cuma buat mempercepat & fallback-kan lookup yang
		// SUDAH VALID, bukan buat rate-limit percobaan API key salah (itu
		// tanggung jawab rate limiter, concern terpisah).
		return nil, err
	}

	if payload, jsonErr := json.Marshal(project); jsonErr == nil {
		// Best-effort -- kegagalan SET ke cache tidak boleh menggagalkan
		// request (project sudah ketemu dari DB, caching cuma optimasi
		// buat request berikutnya).
		_ = r.client.Set(ctx, key, payload, projectCacheTTL).Err()
	}

	return project, nil
}

func (r *CachedProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	return r.inner.Create(ctx, p)
}

func (r *CachedProjectRepository) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	return r.inner.GetByID(ctx, id)
}

func (r *CachedProjectRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.Project, error) {
	return r.inner.ListByUserID(ctx, userID)
}

func (r *CachedProjectRepository) Delete(ctx context.Context, id string) error {
	// Invalidate cache SEGERA saat delete lewat API -- window staleness
	// jadi cuma berlaku buat kasus DI LUAR alur normal (misal manipulasi
	// data langsung di DB), bukan buat delete lewat DELETE /projects/:id
	// yang memang jalur resminya.
	if p, err := r.inner.GetByID(ctx, id); err == nil {
		_ = r.client.Del(ctx, cacheKeyForAPIKeyHash(p.APIKeyHash)).Err()
	}
	return r.inner.Delete(ctx, id)
}
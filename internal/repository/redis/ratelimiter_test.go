package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/testutil"
)

func TestRateLimiter_Allow_WithinLimit(t *testing.T) {
	client := testutil.NewRedisClient(t)
	limiter := redis.NewRateLimiter(client, "test:within", 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		allowed, err := limiter.Allow(ctx, "user-1")
		assert.NoError(t, err)
		assert.True(t, allowed, "request ke-%d harus diizinkan (masih dalam limit)", i+1)
	}
}

func TestRateLimiter_Allow_ExceedsLimit(t *testing.T) {
	client := testutil.NewRedisClient(t)
	limiter := redis.NewRateLimiter(client, "test:exceeds", 3, time.Minute)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_, err := limiter.Allow(ctx, "user-1")
		assert.NoError(t, err)
	}

	allowed, err := limiter.Allow(ctx, "user-1")
	assert.NoError(t, err)
	assert.False(t, allowed, "request ke-4 harus ditolak (melebihi limit 3)")
}

// TestRateLimiter_Allow_DifferentKeysIndependent — bukti key benar-benar
// isolated per identity, bukan cuma per prefix. Kalau ini gagal, artinya
// ada bug serius: user A yang spam bakal ikut nge-block user B.
func TestRateLimiter_Allow_DifferentKeysIndependent(t *testing.T) {
	client := testutil.NewRedisClient(t)
	limiter := redis.NewRateLimiter(client, "test:independent", 2, time.Minute)
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		_, err := limiter.Allow(ctx, "user-A")
		assert.NoError(t, err)
	}
	allowedA, _ := limiter.Allow(ctx, "user-A")
	assert.False(t, allowedA, "user-A sudah kehabisan limit")

	allowedB, err := limiter.Allow(ctx, "user-B")
	assert.NoError(t, err)
	assert.True(t, allowedB, "user-B belum pernah request, harus diizinkan, TIDAK terpengaruh user-A")
}

// TestRateLimiter_Allow_ResetsAfterWindow — nguji perilaku sliding window
// counter LEWAT WAKTU ASLI (bukan di-mock). Window 1 DETIK (bukan 500ms
// seperti percobaan awal) — window di bawah 1 detik memicu panic
// divide-by-zero di algoritma (lihat guard baru di NewRateLimiter), jadi
// 1 detik ini sekaligus jadi batas bawah realistis, bukan angka bebas.
func TestRateLimiter_Allow_ResetsAfterWindow(t *testing.T) {
	client := testutil.NewRedisClient(t)
	limiter := redis.NewRateLimiter(client, "test:reset", 1, time.Second)
	ctx := context.Background()

	allowed1, err := limiter.Allow(ctx, "user-1")
	assert.NoError(t, err)
	assert.True(t, allowed1)

	allowed2, err := limiter.Allow(ctx, "user-1")
	assert.NoError(t, err)
	assert.False(t, allowed2, "masih dalam window yang sama, limit 1 sudah habis")

	// Tunggu lebih dari 2x window — weight window lama di algoritma
	// weighted-average harus sudah negligible di titik ini.
	time.Sleep(2100 * time.Millisecond)

	allowed3, err := limiter.Allow(ctx, "user-1")
	assert.NoError(t, err)
	assert.True(t, allowed3, "setelah window baru, request harus diizinkan lagi")
}

// TestNewRateLimiter_PanicsOnSubSecondWindow — bukti eksplisit guard di
// NewRateLimiter bekerja, bukan cuma asumsi "harusnya begitu". Ini juga
// dokumentasi hidup: siapapun baca test ini langsung tahu batasannya,
// tanpa perlu re-derive dari baca algoritma allow() satu-satu.
func TestNewRateLimiter_PanicsOnSubSecondWindow(t *testing.T) {
	client := testutil.NewRedisClient(t)

	assert.Panics(t, func() {
		redis.NewRateLimiter(client, "test:invalid", 1, 500*time.Millisecond)
	}, "constructor harus panic untuk window di bawah 1 detik")
}
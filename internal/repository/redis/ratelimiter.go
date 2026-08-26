package redis

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter mengimplementasikan domain.RateLimiter dengan sliding window
// counter algorithm — weighted average window sekarang & window sebelumnya.
// Ini approximation (bukan sliding log 100% akurat yang butuh ZADD per
// request, memory tumbuh linear per traffic) — cukup buat fix boundary-burst
// issue fixed-window tanpa kompleksitas Lua-script token bucket yang tidak
// diperlukan untuk semantik "N req per window" (YAGNI, Sprint 9).
//
// Konfigurasi (prefix, limit, window) di-bind saat construction — beda
// kebutuhan (ingest per API key, login per IP, login per email) dibuat
// sebagai instance TERPISAH dari struct yang sama, bukan lewat interface
// baru atau method tambahan (lihat cmd/api/main.go wiring).
type RateLimiter struct {
	client *redis.Client
	prefix string
	limit  int
	window time.Duration
}

func NewRateLimiter(client *redis.Client, prefix string, limit int, window time.Duration) *RateLimiter {
	// Guard wajib — algoritma allow() pakai int64(window.Seconds()) buat
	// hitung window ID, window di bawah 1 detik ke-truncate jadi 0 →
	// integer divide-by-zero waktu request pertama masuk (ditemukan lewat
	// repository-level test, bukan production incident — tapi tetap perlu
	// dicegah eksplisit, bukan cuma "kebetulan semua config sekarang aman").
	// Panic di constructor (bukan return error) SENGAJA — semua pemanggilan
	// ini terjadi di main.go saat startup wiring, konsisten sama pola
	// fail-fast yang sudah ada di config.Load() (log.Fatalf kalau env var
	// wajib kosong). Ini programmer error (salah unit di kode), bukan
	// kegagalan eksternal yang perlu di-handle graceful.
	if window < time.Second {
		panic(fmt.Sprintf(
			"ratelimiter: window harus minimal 1 detik, dapat %s (prefix=%q) — window di bawah 1 detik menyebabkan integer divide-by-zero di allow()",
			window, prefix,
		))
	}
	return &RateLimiter{client: client, prefix: prefix, limit: limit, window: window}
}

// Allow — implementasi domain.RateLimiter.
func (r *RateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	now := time.Now().UTC()
	windowSecs := int64(r.window.Seconds())
	currentWindowID := now.Unix() / windowSecs
	previousWindowID := currentWindowID - 1

	currentKey := fmt.Sprintf("%s:%s:%d", r.prefix, key, currentWindowID)
	previousKey := fmt.Sprintf("%s:%s:%d", r.prefix, key, previousWindowID)

	pipe := r.client.Pipeline()
	incrCmd := pipe.Incr(ctx, currentKey)
	getCmd := pipe.Get(ctx, previousKey)
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return false, fmt.Errorf("ratelimiter pipeline exec: %w", err)
	}

	currentCount := incrCmd.Val()
	if currentCount == 1 {
		// Window baru — TTL 2x window supaya key ini masih kebaca sebagai
		// "previous window" oleh request di window berikutnya.
		if err := r.client.Expire(ctx, currentKey, r.window*2).Err(); err != nil {
			return false, fmt.Errorf("ratelimiter expire: %w", err)
		}
	}

	var previousCount int64
	if err := getCmd.Err(); err == nil {
		previousCount, _ = strconv.ParseInt(getCmd.Val(), 10, 64)
	} else if err != redis.Nil {
		return false, fmt.Errorf("ratelimiter get previous: %w", err)
	}

	elapsedInCurrentWindow := now.Unix() % windowSecs
	weight := 1 - float64(elapsedInCurrentWindow)/float64(windowSecs)
	weightedCount := float64(previousCount)*weight + float64(currentCount)

	// Trade-off disengaja: INCR selalu jalan duluan (atomic) SEBELUM
	// weighted count dicek — request yang ujungnya di-deny tetap ke-count.
	// Race condition kecil di boundary saat concurrency tinggi, diterima
	// demi menghindari Lua script custom.
	return weightedCount <= float64(r.limit), nil
}

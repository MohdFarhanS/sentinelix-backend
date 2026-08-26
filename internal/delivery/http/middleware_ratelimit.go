package http

import (
	"net/http"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// DashboardRateLimitMiddleware — cap generik 300 req/menit per user untuk
// SEMUA endpoint dashboard terautentikasi (02-REQUIREMENTS.md NFR-5,
// 04-API-DESIGN.md §10). Beda dari rate limiter login/ingest (business-
// specific, di-inject ke usecase karena logic-nya nempel ke alur bisnis
// tertentu), limit ini SERAGAM lintas endpoint apapun — cuma peduli
// identitas user yang sudah terautentikasi, itu transport-layer concern,
// jadi tempatnya di sini, bukan di-thread ke tiap usecase satu-satu.
//
// HARUS dipasang SETELAH AuthMiddleware di chain — bergantung pada
// UserIDFromContext yang diisi AuthMiddleware.
func DashboardRateLimitMiddleware(limiter domain.RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := UserIDFromContext(r.Context())
			if !ok {
				// Harusnya tidak pernah kejadian (AuthMiddleware sudah
				// jalan duluan, pasti reject kalau user_id gak ada) —
				// tapi kalau ordering middleware suatu saat ke-reorder
				// tidak sengaja, fail CLOSED (401), bukan fail open
				// (lolosin request tanpa rate limit sama sekali).
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
				return
			}

			allowed, err := limiter.Allow(r.Context(), userID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
				return
			}
			if !allowed {
				writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
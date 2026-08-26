package http

import (
	"context"
	"net/http"

	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
)

type contextKey string

const userIDContextKey contextKey = "user_id"

// AccessTokenCookieName harus sama dengan yang di-set di handler_auth.go Login.
const AccessTokenCookieName = "access_token"

// RefreshTokenCookieName — cookie terpisah dari access token, Path
// di-scope ke /api/v1/auth saja (lihat handler_auth.go), TIDAK ikut
// nempel di setiap request dashboard.
const RefreshTokenCookieName = "refresh_token"

// AuthMiddleware baca JWT dari httpOnly cookie (bukan header Authorization,
// sesuai keputusan: token disimpan di httpOnly cookie di sisi client).
func AuthMiddleware(jwtManager *jwt.Manager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(AccessTokenCookieName)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Access token not found")
				return
			}

			claims, err := jwtManager.Verify(cookie.Value)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid or expired token")
				return
			}

			ctx := context.WithValue(r.Context(), userIDContextKey, claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func UserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDContextKey).(string)
	return userID, ok
}

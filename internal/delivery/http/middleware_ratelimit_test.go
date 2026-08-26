package http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockRateLimiter — implementasi palsu domain.RateLimiter, khusus file
// test ini (TIDAK reuse mock dari internal/usecase — beda package, mock
// testify/mock tidak bisa dishare lintas package tanpa diekspor, dan
// export cuma buat 1 file test lain bukan abstraksi yang sepadan).
type mockRateLimiter struct{ mock.Mock }

func (m *mockRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

func TestDashboardRateLimitMiddleware_Allowed(t *testing.T) {
	limiter := new(mockRateLimiter)
	limiter.On("Allow", mock.Anything, "user-1").Return(true, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := DashboardRateLimitMiddleware(limiter)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "user-1"))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	assert.True(t, called, "next handler harus terpanggil kalau limiter mengizinkan")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDashboardRateLimitMiddleware_RateLimited(t *testing.T) {
	limiter := new(mockRateLimiter)
	limiter.On("Allow", mock.Anything, "user-1").Return(false, nil)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	mw := DashboardRateLimitMiddleware(limiter)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "user-1"))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	assert.False(t, called, "next handler TIDAK BOLEH terpanggil kalau limiter menolak")
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
}

// TestDashboardRateLimitMiddleware_NoUserIDInContext_FailsClosed —
// simulasikan skenario AuthMiddleware ke-skip/ke-reorder tidak sengaja
// (userIDContextKey SENGAJA tidak di-set). Middleware harus fail CLOSED
// (401), dan yang lebih penting: limiter.Allow TIDAK BOLEH sampai
// terpanggil sama sekali — bukan cuma soal response code, tapi soal
// tidak buang-buang 1 Redis roundtrip buat request yang sudah pasti
// ditolak duluan.
func TestDashboardRateLimitMiddleware_NoUserIDInContext_FailsClosed(t *testing.T) {
	limiter := new(mockRateLimiter)

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	mw := DashboardRateLimitMiddleware(limiter)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	assert.False(t, called)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	limiter.AssertNotCalled(t, "Allow", mock.Anything, mock.Anything)
}

func TestDashboardRateLimitMiddleware_LimiterError(t *testing.T) {
	limiter := new(mockRateLimiter)
	limiter.On("Allow", mock.Anything, "user-1").Return(false, errors.New("redis connection refused"))

	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	mw := DashboardRateLimitMiddleware(limiter)(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDContextKey, "user-1"))
	rec := httptest.NewRecorder()

	mw.ServeHTTP(rec, req)

	assert.False(t, called, "kalau Redis down, request TIDAK BOLEH tetap lolos ke next handler")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}
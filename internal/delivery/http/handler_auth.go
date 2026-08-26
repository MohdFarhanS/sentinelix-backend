package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

// refreshTokenCookiePath — SENGAJA beda dari Path access token ("/").
// Refresh token cuma perlu dikirim browser ke endpoint /auth/refresh &
// /auth/logout, jadi Path-nya dipersempit ke situ — mengecilkan attack
// surface (makin jarang token sensitif ini "ikut lewat" di request lain).
const refreshTokenCookiePath = "/api/v1/auth"

type AuthHandler struct {
	authUsecase  *usecase.AuthUsecase
	secureCookie bool
}

func NewAuthHandler(authUsecase *usecase.AuthUsecase, secureCookie bool) *AuthHandler {
	return &AuthHandler{authUsecase: authUsecase, secureCookie: secureCookie}
}

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	resp := errorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	writeJSON(w, status, resp)
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// setAuthCookies & clearAuthCookies dipusatkan di sini — dipakai Login,
// Refresh, DAN error path Refresh (token invalid tetap harus dibersihkan
// dari browser, bukan dibiarkan nyangkut) — hindari duplikasi http.SetCookie
// 4x di tempat berbeda dengan risiko field-nya beda-beda tipis (misal lupa
// Secure di salah satu).
func (h *AuthHandler) setAuthCookies(w http.ResponseWriter, out *usecase.LoginOutput) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    out.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(out.ExpiresIn),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     RefreshTokenCookieName,
		Value:    out.RefreshToken,
		Path:     refreshTokenCookiePath,
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(out.RefreshExpiresIn),
	})
}

func (h *AuthHandler) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: AccessTokenCookieName, Value: "", Path: "/",
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	http.SetCookie(w, &http.Cookie{
		Name: RefreshTokenCookieName, Value: "", Path: refreshTokenCookiePath,
		HttpOnly: true, Secure: h.secureCookie, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	out, err := h.authUsecase.Register(r.Context(), usecase.RegisterInput{
		Email:    req.Email,
		Password: req.Password,
	})
	if err != nil {
		var pwErr *domain.PasswordValidationError
		if errors.As(err, &pwErr) {
			writeError(w, http.StatusBadRequest, "WEAK_PASSWORD", pwErr.Error())
			return
		}
		if errors.Is(err, domain.ErrEmailAlreadyExist) {
			writeError(w, http.StatusConflict, "EMAIL_EXISTS", "Email is already registered")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":    out.ID,
		"email": out.Email,
	})
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	out, err := h.authUsecase.Login(r.Context(), usecase.LoginInput{
		Email:    req.Email,
		Password: req.Password,
		IP:       extractClientIP(r),
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidCredential):
			writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIAL", "Invalid email or password")
		case errors.Is(err, usecase.ErrRateLimited):
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many login attempts, please try again later")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
		}
		return
	}

	h.setAuthCookies(w, out)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": out.AccessToken,
		"expires_in":   out.ExpiresIn,
	})
}

// Refresh — endpoint baru (Sprint 9). Baca refresh token dari cookie
// (BUKAN dari body — refresh token tidak pernah dikirim balik ke client
// lewat JSON, cuma lewat httpOnly cookie), validasi & rotasi lewat
// usecase, set cookie baru.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(RefreshTokenCookieName)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Refresh token not found")
		return
	}

	out, err := h.authUsecase.Refresh(r.Context(), cookie.Value)
	if err != nil {
		// Token invalid/expired/revoked — bersihkan cookie di browser juga,
		// jangan biarkan client terus-terusan ngirim token yang sudah pasti
		// ditolak.
		h.clearAuthCookies(w)
		switch {
		case errors.Is(err, domain.ErrRefreshTokenNotFound),
			errors.Is(err, domain.ErrRefreshTokenExpired),
			errors.Is(err, domain.ErrRefreshTokenRevoked):
			writeError(w, http.StatusUnauthorized, "INVALID_REFRESH_TOKEN", "Session expired, please login again")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
		}
		return
	}

	h.setAuthCookies(w, out)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"access_token": out.AccessToken,
		"expires_in":   out.ExpiresIn,
	})
}

// Logout — sekarang revoke refresh token di DB (Sprint 9), bukan cuma
// hapus cookie di browser. Tetap tidak butuh AuthMiddleware (idempotent,
// aman dipanggil walau access token sudah invalid/expired) — kalau
// refresh token cookie tidak ada/sudah invalid, tetap dianggap "berhasil
// logout" (best-effort, error dari usecase sengaja diabaikan di sini).
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(RefreshTokenCookieName); err == nil {
		_ = h.authUsecase.Logout(r.Context(), cookie.Value)
	}
	h.clearAuthCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type AuthHandler struct {
	authUsecase  *usecase.AuthUsecase
	secureCookie bool
}

func NewAuthHandler(authUsecase *usecase.AuthUsecase, secureCookie bool) *AuthHandler {
	return &AuthHandler{authUsecase: authUsecase, secureCookie: secureCookie}
}

type registerRequest struct {
	Email		string `json:"email"`
	Password	string `json:"password"`
}

type errorResponse struct {
	Error struct {
		Code	string `json:"code"`
		Message	string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := errorResponse{}
	resp.Error.Code = code
	resp.Error.Message = message
	json.NewEncoder(w).Encode(resp)
}

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	out, err := h.authUsecase.Register(r.Context(), usecase.RegisterInput{
		Email:		req.Email,
		Password:	req.Password,
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"id":		out.ID,
		"email":	out.Email,
	})
}

type loginRequest struct {
	Email		string `json:"email"`
	Password	string `json:"password"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	out, err := h.authUsecase.Login(r.Context(), usecase.LoginInput{
		Email:		req.Email,
		Password:	req.Password,
	})
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCredential) {
			writeError(w, http.StatusUnauthorized, "INVALID_CREDENTIAL", "Invalid email or password")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    out.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(out.ExpiresIn),
	})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"access_token": out.AccessToken,
		"expires_in": out.ExpiresIn,
	})
}

// Logout hapus cookie access_token di browser. Sengaja tidak butuh AuthMiddleware
// (tidak perlu verifikasi token dulu buat "boleh logout") — clear cookie itu
// operasi idempotent, aman dipanggil walau token sudah invalid/expired.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     AccessTokenCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   h.secureCookie,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // MaxAge negatif = suruh browser hapus cookie ini sekarang juga
	})
	w.WriteHeader(http.StatusNoContent)
}
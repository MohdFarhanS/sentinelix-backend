package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
	"golang.org/x/crypto/bcrypt"
)

// RefreshTokenTTL — 30 hari, disepakati sebagai durasi "tetap login" tanpa
// perlu login ulang tiap hari (Sprint 9). Access token sengaja jauh lebih
// pendek (15 menit, lihat main.go wiring) — refresh flow inilah yang bikin
// access token pendek itu tidak mengganggu UX.
const RefreshTokenTTL = 30 * 24 * time.Hour

type AuthUsecase struct {
	userRepo         domain.UserRepository
	refreshTokenRepo domain.RefreshTokenRepository
	jwtManager       *jwt.Manager
	ipLimiter        domain.RateLimiter
	emailLimiter     domain.RateLimiter
}

func NewAuthUsecase(
	userRepo domain.UserRepository,
	refreshTokenRepo domain.RefreshTokenRepository,
	jwtManager *jwt.Manager,
	ipLimiter domain.RateLimiter,
	emailLimiter domain.RateLimiter,
) *AuthUsecase {
	return &AuthUsecase{
		userRepo:         userRepo,
		refreshTokenRepo: refreshTokenRepo,
		jwtManager:       jwtManager,
		ipLimiter:        ipLimiter,
		emailLimiter:     emailLimiter,
	}
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// generateRefreshToken — random opaque token (BUKAN JWT), pola sama persis
// seperti generateAPIKey di project.go: plaintext dikembalikan sekali ke
// caller, hash-nya yang disimpan ke DB.
func generateRefreshToken() (plaintext, hash string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plaintext = hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

type RegisterInput struct {
	Email    string
	Password string
}

type RegisterOutput struct {
	ID    string
	Email string
}

func (uc *AuthUsecase) Register(ctx context.Context, in RegisterInput) (*RegisterOutput, error) {
	if pwErrs := domain.ValidatePassword(in.Password); len(pwErrs) > 0 {
		return nil, &domain.PasswordValidationError{Errors: pwErrs}
	}

	email := normalizeEmail(in.Email)

	existing, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil && err != domain.ErrUserNotFound {
		return nil, err
	}
	if existing != nil {
		return nil, domain.ErrEmailAlreadyExist
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &domain.User{
		Email:        email,
		PasswordHash: string(hash),
	}
	if err := uc.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}

	return &RegisterOutput{ID: user.ID, Email: user.Email}, nil
}

type LoginInput struct {
	Email    string
	Password string
	IP       string
}

type LoginOutput struct {
	AccessToken      string
	ExpiresIn        int64
	RefreshToken     string
	RefreshExpiresIn int64
}

func (uc *AuthUsecase) Login(ctx context.Context, in LoginInput) (*LoginOutput, error) {
	if allowed, err := uc.ipLimiter.Allow(ctx, in.IP); err != nil {
		return nil, err
	} else if !allowed {
		return nil, ErrRateLimited
	}

	email := normalizeEmail(in.Email)
	if allowed, err := uc.emailLimiter.Allow(ctx, email); err != nil {
		return nil, err
	} else if !allowed {
		return nil, ErrRateLimited
	}

	user, err := uc.userRepo.FindByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrUserNotFound {
			return nil, domain.ErrInvalidCredential
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(in.Password)); err != nil {
		return nil, domain.ErrInvalidCredential
	}

	return uc.issueTokens(ctx, user.ID)
}

// Refresh — validasi refresh token, ROTASI (token lama di-revoke, token
// baru diterbitkan), return access token baru. Kalau token yang dikirim
// ternyata SUDAH revoked sebelumnya, itu diperlakukan sebagai indikasi
// pencurian (dua pihak balapan pakai token yang sama) — semua sesi aktif
// user ini langsung di-invalidate sebagai respons defensif, bukan cuma
// token yang kena reuse.
func (uc *AuthUsecase) Refresh(ctx context.Context, rawToken string) (*LoginOutput, error) {
	sum := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(sum[:])

	stored, err := uc.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		return nil, err // termasuk domain.ErrRefreshTokenNotFound
	}

	if stored.RevokedAt != nil {
		_ = uc.refreshTokenRepo.RevokeAllByUserID(ctx, stored.UserID)
		return nil, domain.ErrRefreshTokenRevoked
	}
	if time.Now().UTC().After(stored.ExpiresAt) {
		return nil, domain.ErrRefreshTokenExpired
	}

	if err := uc.refreshTokenRepo.Revoke(ctx, stored.ID); err != nil {
		return nil, err
	}

	return uc.issueTokens(ctx, stored.UserID)
}

// Logout — revoke refresh token di DB (bukan cuma hapus cookie di
// browser seperti sebelumnya). Sengaja idempotent & tidak pernah return
// error ke caller yang signifikan — token tidak ketemu/sudah revoked
// dianggap "sudah logout", bukan kegagalan.
func (uc *AuthUsecase) Logout(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(rawToken))
	tokenHash := hex.EncodeToString(sum[:])

	stored, err := uc.refreshTokenRepo.FindByTokenHash(ctx, tokenHash)
	if err != nil {
		if err == domain.ErrRefreshTokenNotFound {
			return nil
		}
		return err
	}
	return uc.refreshTokenRepo.Revoke(ctx, stored.ID)
}

// issueTokens — dipakai bersama oleh Login & Refresh, satu tempat buat
// generate access token (JWT) + refresh token (opaque, disimpan ke DB).
func (uc *AuthUsecase) issueTokens(ctx context.Context, userID string) (*LoginOutput, error) {
	accessToken, expiresIn, err := uc.jwtManager.Generate(userID)
	if err != nil {
		return nil, err
	}

	rawRefreshToken, refreshHash, err := generateRefreshToken()
	if err != nil {
		return nil, err
	}

	refreshRecord := &domain.RefreshToken{
		UserID:    userID,
		TokenHash: refreshHash,
		ExpiresAt: time.Now().UTC().Add(RefreshTokenTTL),
	}
	if err := uc.refreshTokenRepo.Create(ctx, refreshRecord); err != nil {
		return nil, err
	}

	return &LoginOutput{
		AccessToken:      accessToken,
		ExpiresIn:        expiresIn,
		RefreshToken:     rawRefreshToken,
		RefreshExpiresIn: int64(RefreshTokenTTL.Seconds()),
	}, nil
}

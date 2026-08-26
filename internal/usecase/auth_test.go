package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
)

// mockUserReporsitory adalah implemenatasi palsu dari domain.UserRepository,
// datanya cuma disimpan di map (memory), bukan Postgres benaran.
type mockUserReporsitory struct {
	users map[string]*domain.User // key = email
}

func newMockUserRepository() *mockUserReporsitory {
	return &mockUserReporsitory{users: make(map[string]*domain.User)}
}

func (m *mockUserReporsitory) Create(ctx context.Context, user *domain.User) error {
	user.ID = "mock-id-123"
	m.users[user.Email] = user
	return nil
}

func (m *mockUserReporsitory) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	user, ok := m.users[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return user, nil
}

// mockRateLimiter TIDAK dideklarasikan di file ini — sudah ada di
// ingest_event_test.go (package usecase_test yang sama, embed mock.Mock).

// mockRefreshTokenRepo — implementasi palsu domain.RefreshTokenRepository.
// Pakai testify/mock (bukan hand-written) karena beberapa test di bawah
// perlu assert ARGUMENT spesifik yang diterima (misal: Revoke dipanggil
// dengan ID token yang benar, RevokeAllByUserID dipanggil dengan userID
// yang benar saat reuse detection ke-trigger).
type mockRefreshTokenRepo struct{ mock.Mock }

func (m *mockRefreshTokenRepo) Create(ctx context.Context, token *domain.RefreshToken) error {
	args := m.Called(ctx, token)
	return args.Error(0)
}

func (m *mockRefreshTokenRepo) FindByTokenHash(ctx context.Context, tokenHash string) (*domain.RefreshToken, error) {
	args := m.Called(ctx, tokenHash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.RefreshToken), args.Error(1)
}

func (m *mockRefreshTokenRepo) Revoke(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockRefreshTokenRepo) RevokeAllByUserID(ctx context.Context, userID string) error {
	args := m.Called(ctx, userID)
	return args.Error(0)
}

// setupAuthUsecase — ipAllow & emailAllow ngontrol apakah mock rate limiter
// meloloskan request. refreshTokenRepo di-stub Create() supaya sukses by
// default (jalur normal Login harus bisa nerbitin refresh token tanpa
// perlu tiap test nge-stub ini manual). Return semua mock yang mungkin
// perlu di-assert lebih lanjut oleh test tertentu.
func setupAuthUsecase(ipAllow, emailAllow bool) (*usecase.AuthUsecase, *mockRateLimiter, *mockRateLimiter, *mockRefreshTokenRepo) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)

	ipLimiter := new(mockRateLimiter)
	ipLimiter.On("Allow", mock.Anything, mock.Anything).Return(ipAllow, nil)

	emailLimiter := new(mockRateLimiter)
	emailLimiter.On("Allow", mock.Anything, mock.Anything).Return(emailAllow, nil)

	refreshTokenRepo := new(mockRefreshTokenRepo)
	refreshTokenRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)
	return uc, ipLimiter, emailLimiter, refreshTokenRepo
}

// validPassword: penuhi semua rule complexity klasik (min 8, upper, lower, digit, special).
// Dipakai sebagai fixture standar di semua test yang BUKAN spesifik nguji validasi password.
const validPassword = "Secret123!"

func TestRegister_Success(t *testing.T) {
	uc, _, _, _ := setupAuthUsecase(true, true)

	out, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})

	require.NoError(t, err)
	assert.Equal(t, "test@sentinelix.com", out.Email)
	assert.NotEmpty(t, out.ID)
}

func TestRegister_EmailAlreadyExist(t *testing.T) {
	uc, _, _, _ := setupAuthUsecase(true, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	_, err = uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: "Different456!",
	})

	assert.ErrorIs(t, err, domain.ErrEmailAlreadyExist)
}

func TestRegister_WeakPassword(t *testing.T) {
	uc, _, _, _ := setupAuthUsecase(true, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: "weak",
	})

	require.Error(t, err)
	var pwErr *domain.PasswordValidationError
	assert.ErrorAs(t, err, &pwErr)
}

func TestLogin_Success(t *testing.T) {
	uc, _, _, refreshTokenRepo := setupAuthUsecase(true, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	out, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
		IP:       "127.0.0.1",
	})

	require.NoError(t, err)
	assert.NotEmpty(t, out.AccessToken)
	assert.Equal(t, int64(3600), out.ExpiresIn)
	// NEW (Sprint 9): Login sekarang juga nerbitin refresh token.
	assert.NotEmpty(t, out.RefreshToken)
	assert.Equal(t, int64(usecase.RefreshTokenTTL.Seconds()), out.RefreshExpiresIn)
	refreshTokenRepo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestLogin_WrongPassword(t *testing.T) {
	uc, _, _, _ := setupAuthUsecase(true, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	_, err = uc.Login(context.Background(), usecase.LoginInput{
		Email:    "test@sentinelix.com",
		Password: "wrongpassword",
		IP:       "127.0.0.1",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
}

func TestLogin_EmailNotFound(t *testing.T) {
	uc, _, _, _ := setupAuthUsecase(true, true)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "notexist@sentinelix.com",
		Password: "secret123",
		IP:       "127.0.0.1",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
}

func TestLogin_RateLimitedByIP(t *testing.T) {
	uc, _, _, refreshTokenRepo := setupAuthUsecase(false, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	_, err = uc.Login(context.Background(), usecase.LoginInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
		IP:       "1.2.3.4",
	})

	assert.ErrorIs(t, err, usecase.ErrRateLimited)
	// Ditolak sebelum sempat sampai issueTokens — refresh token tidak
	// boleh ke-generate buat percobaan yang di-rate-limit.
	refreshTokenRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestLogin_RateLimitedByEmail(t *testing.T) {
	uc, _, _, refreshTokenRepo := setupAuthUsecase(true, false)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	_, err = uc.Login(context.Background(), usecase.LoginInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
		IP:       "1.2.3.4",
	})

	assert.ErrorIs(t, err, usecase.ErrRateLimited)
	refreshTokenRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestLogin_EmailNormalization(t *testing.T) {
	uc, _, emailLimiter, _ := setupAuthUsecase(true, true)

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
	})
	require.NoError(t, err)

	_, err = uc.Login(context.Background(), usecase.LoginInput{
		Email:    "  Test@SentinelIX.com  ",
		Password: validPassword,
		IP:       "1.2.3.4",
	})

	require.NoError(t, err)
	emailLimiter.AssertCalled(t, "Allow", mock.Anything, "test@sentinelix.com")
}

func TestLogin_RateLimiterError(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)

	ipLimiter := new(mockRateLimiter)
	ipLimiter.On("Allow", mock.Anything, mock.Anything).Return(false, errors.New("redis connection refused"))
	emailLimiter := new(mockRateLimiter)
	emailLimiter.On("Allow", mock.Anything, mock.Anything).Return(true, nil)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email:    "test@sentinelix.com",
		Password: validPassword,
		IP:       "1.2.3.4",
	})

	require.Error(t, err)
	assert.NotErrorIs(t, err, domain.ErrInvalidCredential)
	assert.NotErrorIs(t, err, usecase.ErrRateLimited)
	emailLimiter.AssertNotCalled(t, "Allow", mock.Anything, mock.Anything)
	refreshTokenRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// NEW (Sprint 9): AuthUsecase.Refresh — validasi & rotasi refresh token.

func TestRefresh_Success(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	existing := &domain.RefreshToken{
		ID:        "rt-1",
		UserID:    "user-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
		RevokedAt: nil,
	}
	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).Return(existing, nil)
	refreshTokenRepo.On("Revoke", mock.Anything, "rt-1").Return(nil)
	refreshTokenRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	out, err := uc.Refresh(context.Background(), "some-raw-refresh-token")

	require.NoError(t, err)
	assert.NotEmpty(t, out.AccessToken)
	assert.NotEmpty(t, out.RefreshToken)
	// Rotasi: token lama di-revoke, token baru diterbitkan — DUA-DUANYA
	// harus terjadi, bukan salah satu.
	refreshTokenRepo.AssertCalled(t, "Revoke", mock.Anything, "rt-1")
	refreshTokenRepo.AssertCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestRefresh_TokenNotFound(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).
		Return(nil, domain.ErrRefreshTokenNotFound)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	_, err := uc.Refresh(context.Background(), "nonexistent-token")

	assert.ErrorIs(t, err, domain.ErrRefreshTokenNotFound)
}

func TestRefresh_TokenExpired(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	expired := &domain.RefreshToken{
		ID:        "rt-1",
		UserID:    "user-1",
		ExpiresAt: time.Now().UTC().Add(-time.Hour), // sudah lewat
		RevokedAt: nil,
	}
	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).Return(expired, nil)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	_, err := uc.Refresh(context.Background(), "expired-token")

	assert.ErrorIs(t, err, domain.ErrRefreshTokenExpired)
	refreshTokenRepo.AssertNotCalled(t, "Revoke", mock.Anything, mock.Anything)
}

// TestRefresh_ReuseDetection_RevokesAllSessions — token yang SUDAH pernah
// revoked (dipakai ulang) memicu semua sesi user itu di-invalidate
// sekaligus, bukan cuma token yang kena reuse.
func TestRefresh_ReuseDetection_RevokesAllSessions(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	revokedAt := time.Now().UTC().Add(-time.Minute)
	alreadyRevoked := &domain.RefreshToken{
		ID:        "rt-1",
		UserID:    "user-1",
		ExpiresAt: time.Now().UTC().Add(time.Hour), // secara waktu belum expired
		RevokedAt: &revokedAt,                      // tapi SUDAH pernah dipakai
	}
	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).Return(alreadyRevoked, nil)
	refreshTokenRepo.On("RevokeAllByUserID", mock.Anything, "user-1").Return(nil)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	_, err := uc.Refresh(context.Background(), "stolen-token")

	assert.ErrorIs(t, err, domain.ErrRefreshTokenRevoked)
	refreshTokenRepo.AssertCalled(t, "RevokeAllByUserID", mock.Anything, "user-1")
}

// NEW (Sprint 9): AuthUsecase.Logout — revoke refresh token di DB.

func TestLogout_Success(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	existing := &domain.RefreshToken{ID: "rt-1", UserID: "user-1"}
	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).Return(existing, nil)
	refreshTokenRepo.On("Revoke", mock.Anything, "rt-1").Return(nil)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	err := uc.Logout(context.Background(), "some-token")

	require.NoError(t, err)
	refreshTokenRepo.AssertCalled(t, "Revoke", mock.Anything, "rt-1")
}

// TestLogout_TokenNotFound_StillSucceeds — idempotent by design: token
// yang sudah tidak ada (misal logout dobel-klik) dianggap "sudah logout",
// bukan error ke user.
func TestLogout_TokenNotFound_StillSucceeds(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	refreshTokenRepo.On("FindByTokenHash", mock.Anything, mock.Anything).
		Return(nil, domain.ErrRefreshTokenNotFound)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	err := uc.Logout(context.Background(), "already-gone-token")

	assert.NoError(t, err)
	refreshTokenRepo.AssertNotCalled(t, "Revoke", mock.Anything, mock.Anything)
}

func TestLogout_EmptyToken_NoRepoCall(t *testing.T) {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	ipLimiter := new(mockRateLimiter)
	emailLimiter := new(mockRateLimiter)
	refreshTokenRepo := new(mockRefreshTokenRepo)

	uc := usecase.NewAuthUsecase(repo, refreshTokenRepo, jwtManager, ipLimiter, emailLimiter)

	err := uc.Logout(context.Background(), "")

	assert.NoError(t, err)
	refreshTokenRepo.AssertNotCalled(t, "FindByTokenHash", mock.Anything, mock.Anything)
}

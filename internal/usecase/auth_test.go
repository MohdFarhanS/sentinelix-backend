package usecase_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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

func setupAuthUsecase() *usecase.AuthUsecase {
	repo := newMockUserRepository()
	jwtManager := jwt.NewManager("test-secret", time.Hour)
	return usecase.NewAuthUsecase(repo, jwtManager)
}

// validPassword: penuhi semua rule complexity klasik (min 8, upper, lower, digit, special).
// Dipakai sebagai fixture standar di semua test yang BUKAN spesifik nguji validasi password.
const validPassword = "Secret123!"

func TestRegister_Success(t *testing.T) {
	uc := setupAuthUsecase()

	out, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:		"test@devpulse.com",
		Password: 	validPassword,
	})

	require.NoError(t, err)
	assert.Equal(t, "test@devpulse.com", out.Email)
	assert.NotEmpty(t, out.ID)
} 

func TestRegister_EmailAlreadyExist(t *testing.T) {
	uc := setupAuthUsecase()

	// Register pertama kali harus sukses
	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email: 		"test@devpulse.com",
		Password: 	validPassword,
	})
	require.NoError(t, err)

	// Register kedua kali dengan email yang sama harus gagal karena EMAIL,
	// bukan karena password — jadi password di sini TETAP HARUS valid
	// (complexity-compliant), supaya request ini beneran sampai ke
	// pengecekan email-exist, bukan ke-reject duluan di validasi password.
	_, err = uc.Register(context.Background(), usecase.RegisterInput{
		Email: 		"test@devpulse.com",
		Password: 	"Different456!",
	})

	assert.ErrorIs(t, err, domain.ErrEmailAlreadyExist)
}

// NEW: test khusus buat behavior validasi password di level usecase
// (bukan cuma di domain.ValidatePassword yang sudah dites terpisah).
// Ini mastiin Register() BENERAN manggil validasi itu dan meneruskan
// errornya dengan tipe yang benar.
func TestRegister_WeakPassword(t *testing.T) {
	uc := setupAuthUsecase()

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email:		"test@devpulse.com",
		Password: 	"weak",
	})

	require.Error(t, err)
	var pwErr *domain.PasswordValidationError
	assert.ErrorAs(t, err, &pwErr)
}

func TestLogin_Success(t *testing.T) {
	uc := setupAuthUsecase()

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email: 		"test@devpulse.com",
		Password: 	validPassword,
	})
	require.NoError(t, err)

	out, err := uc.Login(context.Background(), usecase.LoginInput{
		Email: 		"test@devpulse.com",
		Password: 	validPassword,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, out.AccessToken)
	assert.Equal(t, int64(3600), out.ExpiresIn)
}

func TestLogin_WrongPassword(t *testing.T) {
	uc := setupAuthUsecase()

	_, err := uc.Register(context.Background(), usecase.RegisterInput{
		Email: 		"test@devpulse.com",
		Password: 	validPassword,
	})
	require.NoError(t, err)

	// Password salah ini TIDAK perlu penuhi complexity — Login tidak
	// menjalankan ValidatePassword sama sekali (memang cuma dicek pas
	// Register). Di sini kita nguji jalur bcrypt.CompareHashAndPassword gagal.
	_, err = uc.Login(context.Background(), usecase.LoginInput{
		Email: 		"test@devpulse.com",
		Password: 	"wrongpassword",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
}

func TestLogin_EmailNotFound(t *testing.T) {
	uc := setupAuthUsecase()

	_, err := uc.Login(context.Background(), usecase.LoginInput{
		Email: 		"notexist@devpulse.com",
		Password: 	"secret123",
	})

	assert.ErrorIs(t, err, domain.ErrInvalidCredential)
}
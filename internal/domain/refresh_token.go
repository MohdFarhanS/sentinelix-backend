package domain

import (
	"context"
	"errors"
	"time"
)

var (
	ErrRefreshTokenNotFound = errors.New("refresh token not found")
	ErrRefreshTokenExpired  = errors.New("refresh token expired")
	ErrRefreshTokenRevoked  = errors.New("refresh token already used or revoked")
)

// RefreshToken — opaque random token (BUKAN JWT), disimpan hash-nya saja
// (pola sama persis seperti Project.APIKeyHash) supaya kalau tabel ini
// bocor, token mentah tidak langsung reusable. TokenHash unik per baris —
// satu user bisa punya banyak baris aktif sekaligus (multi-session/device),
// beda dari access token yang stateless JWT tanpa tabel.
//
// RevokedAt nullable — nil berarti masih aktif. Diisi saat: (1) dipakai
// buat refresh (rotasi — token lama di-revoke, token baru diterbitkan),
// (2) logout eksplisit, atau (3) reuse detection mendeteksi token yang
// sudah revoked dipakai lagi (indikasi dicuri) — lihat
// RefreshTokenUsecase nanti di layer usecase.
type RefreshToken struct {
	ID        string
	UserID    string
	TokenHash string
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// IsValid — helper dipakai usecase, supaya logic "apa itu token yang valid"
// terpusat di satu tempat (domain), bukan diduplikasi di tiap pemanggil.
func (rt *RefreshToken) IsValid() bool {
	return rt.RevokedAt == nil && time.Now().UTC().Before(rt.ExpiresAt)
}

// RefreshTokenRepository didefinisikan di domain, diimplementasikan di
// repository/postgres — pola sama seperti UserRepository.
type RefreshTokenRepository interface {
	// Create menyimpan token baru (dipanggil saat login & saat rotasi).
	Create(ctx context.Context, token *RefreshToken) error

	// FindByTokenHash mengambil satu baris berdasarkan hash token mentah
	// yang dikirim client — dipakai saat /auth/refresh buat validasi.
	FindByTokenHash(ctx context.Context, tokenHash string) (*RefreshToken, error)

	// Revoke menandai satu token sebagai revoked (set revoked_at = now()).
	// Dipakai saat rotasi (token lama) & saat logout eksplisit.
	Revoke(ctx context.Context, id string) error

	// RevokeAllByUserID menandai SEMUA token aktif milik user sebagai
	// revoked — dipakai khusus saat reuse detection ke-trigger (token yang
	// sudah revoked dipakai lagi = indikasi dicuri, invalidate semua sesi
	// user itu sebagai respons defensif, bukan cuma token yang kena reuse).
	RevokeAllByUserID(ctx context.Context, userID string) error
}

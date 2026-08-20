package apikey

import (
	"crypto/sha256"
	"encoding/hex"
)

// Hash menghasilkan SHA-256 hex digest dari API key mentah.
// Dipakai konsisten baik saat lookup (sekarang) maupun nanti saat generate
// API key baru di endpoint POST /projects.
func Hash(rawKey string) string {
	sum := sha256.Sum256([]byte(rawKey))
	return hex.EncodeToString(sum[:])
}
package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

// lineNumberRegex menangkap pola ":<angka>" seperti ":88" atau ":88:12"
// (line:column) yang biasa muncul di stack trace, supaya bisa di-strip
// sebelum hashing.
var lineNumberRegex = regexp.MustCompile(`:\d+`)

// Compute menghasilkan fingerprint deterministik dari message error dan
// stack trace, dipakai untuk grouping event menjadi issue yang sama.
//
// Algoritma v1 ("top 1 frame + normalisasi line number"):
//  1. Ambil baris pertama non-kosong dari stack trace sebagai "top frame" —
//     asumsi: baris pertama = titik error paling relevan.
//  2. Strip semua angka setelah ":" di top frame (line & column number),
//     supaya refactor kecil (nambah/kurang baris kode di atas titik error)
//     tidak bikin fingerprint berubah dan issue lama ke-split jadi issue baru.
//  3. Gabungkan message + top frame yang sudah dinormalisasi.
//  4. Hash pakai SHA-256 → hex string (64 karakter, PAS dengan kolom
//     issues.fingerprint VARCHAR(64) di 03-DATABASE-DESIGN.md — tidak perlu
//     truncate).
//
// Known limitation (sengaja belum di-handle di v1, lihat 06-ROADMAP.md
// bagian "Risiko Teknis" — iterasi berdasarkan hasil testing):
//   - Message yang mengandung data dinamis (mis. "...for user_id 123")
//     akan menghasilkan fingerprint berbeda untuk data yang berbeda,
//     padahal root cause sama.
//   - Bug yang sama dipanggil dari 2 call path berbeda akan dianggap
//     2 issue berbeda (baru ke-cover kalau nanti upgrade ke top-N frame).
func Compute(message, stackTrace string) string {
	topFrame := extractTopFrame(stackTrace)
	normalized := normalizeFrame(topFrame)

	// separator "|" mencegah false-match antara
	// message="AB" + frame="C" vs message="A" + frame="BC"
	raw := message + "|" + normalized
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func extractTopFrame(stackTrace string) string {
	for _, line := range strings.Split(stackTrace, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeFrame(frame string) string {
	return lineNumberRegex.ReplaceAllString(frame, "")
}
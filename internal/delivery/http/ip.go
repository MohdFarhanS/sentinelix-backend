package http

import (
	"net"
	"net/http"
	"strings"
)

// extractClientIP mengambil IP client asli dari header X-Forwarded-For.
//
// Best practice GENERIK untuk XFF sebenarnya ambil IP dari KANAN, mundur
// sejumlah hop proxy tepercaya — karena secara default proxy meng-APPEND
// IP-nya ke akhir list, bukan mengganti apa yang sudah ada, sehingga
// index PERTAMA bisa dispoof client (kirim header XFF palsu sebelum
// mencapai proxy manapun).
//
// TAPI: ini deployment di Render, dan perilaku Render beda dari proxy
// generik. Staff Render mengonfirmasi mereka men-SET ULANG (bukan append)
// index pertama X-Forwarded-For jadi real client IP yang mereka lihat di
// koneksi TCP:
//   https://feedback.render.com/features/p/send-the-correct-xforwardedfor
//   ("we set the first IP in the list to the real client IP")
//
// Karena Render adalah SATU-SATUNYA trust boundary di depan aplikasi ini
// (tidak ada proxy chain lain sebelum request sampai ke Render), ambil
// index pertama aman KHUSUS untuk infrastruktur ini. Kalau suatu saat
// pindah hosting (di belakang Cloudflare + load balancer sendiri, misalnya),
// asumsi ini HARUS diverifikasi ulang — jangan asumsikan otomatis portable.
func extractClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		if ip := strings.TrimSpace(strings.Split(xff, ",")[0]); ip != "" {
			return ip
		}
	}

	// Fallback: local dev — tidak ada proxy Render di depan, RemoteAddr
	// berformat "ip:port", perlu di-split.
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

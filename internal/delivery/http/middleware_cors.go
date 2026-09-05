package http

import (
	"net/http"
	"strings"
)

// CORSMiddleware — SATU middleware global (dipasang r.Use() di root
// router), BUKAN per-route. Ini penting: middleware per-route (r.With())
// di chi baru jalan SETELAH request ditentukan match ke route tertentu —
// request OPTIONS preflight ke /ingest/event tetap akan kena middleware
// GLOBAL ini duluan, jadi behavior beda-per-path harus ditangani DI SINI,
// bukan lewat middleware terpisah yang cuma dipasang di 1 route.
//
// /api/v1/ingest/event SENGAJA dikecualikan dari strict single-origin
// policy: endpoint ini didesain dipanggil dari BROWSER situs pihak ketiga
// (newsportal.my.id, atau website manapun yang pasang instrumentasi
// client-side SentinelIX) — bukan cuma dari dashboard SentinelIX sendiri.
// Autentikasinya API key di header (X-SentinelIX-Key), BUKAN cookie, jadi
// aman allow semua origin di situ — beda dari endpoint dashboard yang
// pakai httpOnly cookie + kredensial, wajib dibatasi ke 1 origin ketat.
func CORSMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/ingest/event") {
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-SentinelIX-Key")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-SentinelIX-Key")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
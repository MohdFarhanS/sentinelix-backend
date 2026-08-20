package http

import "net/http"

// CORSMiddleware manual, sengaja tidak pakai go-chi/cors (dependency baru)
// karena kebutuhan kita cuma satu origin (frontend) — sesuai prinsip YAGNI.
// Access-Control-Allow-Credentials wajib "true" supaya browser mau kirim
// httpOnly cookie di request cross-origin dari Next.js ke API ini.
func CORSMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-SentinelIX-Key")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
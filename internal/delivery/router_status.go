package delivery

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// NewStatusRouter — chi.NewRouter() SENDIRI, TIDAK reuse http.NewRouter()
// dashboard. Endpoint publik, TANPA AuthMiddleware, TANPA CORS whitelist
// dashboard (lihat 05-ARCHITECTURE.md §6c) — status page di-fetch
// server-side dari Next.js ISR, bukan langsung dari browser, jadi CORS
// browser-based tidak relevan di sini.
func NewStatusRouter(StatusHandler *StatusHandler) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// /healthz SENGAJA di root (bukan /api/v1) dan TIDAK query DB sama
	// sekali — target ping keep-warm GitHub Actions cron (lihat
	// 05-ARCHITECTURE.md §6c "Kenapa /healthz tidak boleh menyentuh
	// database").
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/api/v1/status/{slug}", StatusHandler.GetBySlug)

	return r
}
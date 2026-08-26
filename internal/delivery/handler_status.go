package delivery

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type StatusHandler struct {
	getStatusPageUsecase *usecase.GetStatusPageUsecase
	logger               zerolog.Logger
}

func NewStatusHandler(getStatusPageUsecase *usecase.GetStatusPageUsecase, logger zerolog.Logger) *StatusHandler {
	return &StatusHandler{getStatusPageUsecase: getStatusPageUsecase, logger: logger}
}

type statusMonitorResponse struct {
	Name      string  `json:"name"`
	IsUp      bool    `json:"is_up"`
	Uptime30d float64 `json:"uptime_30d"`
}

type statusPageResponseDTO struct {
	ProjectName   string                  `json:"project_name"`
	OverallStatus string                  `json:"overall_status"`
	Monitors      []statusMonitorResponse `json:"monitors"`
}

// writeStatusJSON & writeStatusError SENGAJA duplikat kecil dari
// writeJSON/writeError di internal/delivery/http — package `delivery`
// ini TIDAK BOLEH import package `http` (akan menarik AuthHandler,
// WSHandler, dkk ikut ke dependency graph binary status-api, melanggar
// isolasi NFR-9). Duplikasi ~10 baris ini trade-off yang diambil sadar.
func writeStatusJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeStatusError(w http.ResponseWriter, status int, code, message string) {
	writeStatusJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": message},
	})
}

func (h *StatusHandler) GetBySlug(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")

	result, err := h.getStatusPageUsecase.Execute(r.Context(), slug)
	if err != nil {
		if errors.Is(err, domain.ErrProjectSlugNotFound) {
			writeStatusError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Status page not found")
			return
		}
		h.logger.Error().Err(err).Str("slug", slug).Msg("unhandled get status page error")
		writeStatusError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
		return
	}

	monitors := make([]statusMonitorResponse, 0, len(result.Monitors))
	for _, m := range result.Monitors {
		monitors = append(monitors, statusMonitorResponse{
			Name:      m.Name,
			IsUp:      m.IsUp,
			Uptime30d: m.Uptime30d,
		})
	}

	writeStatusJSON(w, http.StatusOK, statusPageResponseDTO{
		ProjectName:   result.ProjectName,
		OverallStatus: result.OverallStatus,
		Monitors:      monitors,
	})
}

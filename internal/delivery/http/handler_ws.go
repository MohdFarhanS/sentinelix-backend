package http

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/delivery/ws"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type WSHandler struct {
	hub            *ws.Hub
	projectUsecase *usecase.ProjectUsecase
	logger         zerolog.Logger
}

func NewWSHandler(hub *ws.Hub, projectUsecase *usecase.ProjectUsecase, logger zerolog.Logger) *WSHandler {
	return &WSHandler{hub: hub, projectUsecase: projectUsecase, logger: logger}
}

// upgrader.CheckOrigin sengaja dibiarkan default (cuma terima same-origin)
// DITAMBAH pengecekan manual ke FrontendURL, konsisten dengan CORS whitelist
// yang sudah dipasang di CORSMiddleware buat REST endpoint biasa.
func newUpgrader(frontendURL string) websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return r.Header.Get("Origin") == frontendURL
		},
	}
}

func (h *WSHandler) HandleConnection(frontendURL string) http.HandlerFunc {
	upgrader := newUpgrader(frontendURL)

	return func(w http.ResponseWriter, r *http.Request) {
		userID, ok := UserIDFromContext(r.Context())
		if !ok {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
			return
		}

		projectID := chi.URLParam(r, "id")

		if err := h.projectUsecase.VerifyOwnership(r.Context(), userID, projectID); err != nil {
			// NOTE: sebelum Upgrade() dipanggil, response masih bisa
			// pakai writeError biasa (belum jadi WS connection).
			switch {
			case err == usecase.ErrForbidden:
				writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this project")
			default:
				writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
			}
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			h.logger.Error().Err(err).Msg("failed to upgrade to websocket")
			return
		}

		client := ws.NewClient(conn, projectID)
		h.hub.Register(client)

		go client.WritePump()
		go client.ReadPump(h.hub)
	}
}
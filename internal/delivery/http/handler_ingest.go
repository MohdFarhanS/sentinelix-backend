package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type IngestHandler struct {
	uc *usecase.IngestEventUsecase
}

func NewIngestHandler(uc *usecase.IngestEventUsecase) *IngestHandler {
	return &IngestHandler{uc: uc}
}

type ingestEventRequest struct {
	Level      string         `json:"level"`
	Message    string         `json:"message"`
	StackTrace string         `json:"stack_trace"`
	Context    map[string]any `json:"context"`
}

func (h *IngestHandler) HandleIngestEvent(w http.ResponseWriter, r *http.Request) {
	rawKey := r.Header.Get("X-SentinelIX-Key")
	if rawKey == "" {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "X-SentinelIX-Key header is required")
		return
	}

	var req ingestEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Invalid request body")
		return
	}

	err := h.uc.Execute(r.Context(), usecase.IngestEventInput{
		RawAPIKey:  rawKey,
		Level:      req.Level,
		Message:    req.Message,
		StackTrace: req.StackTrace,
		Context:    req.Context,
	})

	switch {
	case err == nil:
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(map[string]bool{"accepted": true})
	case errors.Is(err, usecase.ErrInvalidAPIKey):
		writeError(w, http.StatusUnauthorized, "INVALID_API_KEY", "Invalid API key")
	case errors.Is(err, usecase.ErrRateLimited):
		writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests")
	case errors.Is(err, domain.ErrEventMessageRequired), errors.Is(err, domain.ErrEventLevelInvalid):
		writeError(w, http.StatusBadRequest, "INVALID_PAYLOAD", err.Error())
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong on our end")
	}
}
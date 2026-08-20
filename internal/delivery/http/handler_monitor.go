package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type MonitorHandler struct {
	monitorUsecase *usecase.MonitorUsecase
	logger         zerolog.Logger
}

func NewMonitorHandler(monitorUsecase *usecase.MonitorUsecase, logger zerolog.Logger) *MonitorHandler {
	return &MonitorHandler{monitorUsecase: monitorUsecase, logger: logger}
}

type createMonitorRequest struct {
	URL              string `json:"url"`
	IntervalSec      int    `json:"interval_sec"`
	Channel          string `json:"channel"`
	ChannelTarget    string `json:"channel_target"`
	FailureThreshold int    `json:"failure_threshold"`
}

type monitorResponse struct {
	ID               string `json:"id"`
	ProjectID        string `json:"project_id"`
	URL              string `json:"url"`
	IntervalSec      int    `json:"interval_sec"`
	Channel          string `json:"channel"`
	ChannelTarget    string `json:"channel_target"`
	FailureThreshold int    `json:"failure_threshold"`
	Status           string `json:"status"`
	CreatedAt        string `json:"created_at"`
}

func toMonitorResponse(m *domain.Monitor) monitorResponse {
	return monitorResponse{
		ID:               m.ID,
		ProjectID:        m.ProjectID,
		URL:              m.URL,
		IntervalSec:      m.IntervalSec,
		Channel:          m.Channel,
		ChannelTarget:    m.ChannelTarget,
		FailureThreshold: m.FailureThreshold,
		Status:           m.Status,
		CreatedAt:        m.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// writeMonitorError menyatukan error-mapping yang dipakai berulang di
// semua handler method. default case SELALU di-log ke server (logger)
// sebelum balikin 500 generic ke client — tanpa ini, error asli hilang
// total, susah debug production issue.
func writeMonitorError(w http.ResponseWriter, logger zerolog.Logger, err error) {
	switch {
	case errors.Is(err, domain.ErrMonitorNotFound):
		writeError(w, http.StatusNotFound, "MONITOR_NOT_FOUND", "Monitor not found")
	case errors.Is(err, domain.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
	case errors.Is(err, usecase.ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this monitor")
	case errors.Is(err, domain.ErrMonitorURLRequired),
		errors.Is(err, domain.ErrMonitorURLInvalid),
		errors.Is(err, domain.ErrMonitorIntervalInvalid),
		errors.Is(err, domain.ErrMonitorChannelInvalid),
		errors.Is(err, domain.ErrMonitorChannelTargetRequired),
		errors.Is(err, domain.ErrMonitorFailureThresholdInvalid):
		writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
	default:
		logger.Error().Err(err).Msg("unhandled monitor usecase error")
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Something went wrong")
	}
}

func (h *MonitorHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projectID := chi.URLParam(r, "projectId")

	var req createMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body is not valid JSON")
		return
	}

	if req.FailureThreshold == 0 {
		req.FailureThreshold = domain.DefaultFailureThreshold
	}

	monitor, err := h.monitorUsecase.Create(r.Context(), usecase.CreateMonitorInput{
		UserID:           userID,
		ProjectID:        projectID,
		URL:              req.URL,
		IntervalSec:      req.IntervalSec,
		Channel:          req.Channel,
		ChannelTarget:    req.ChannelTarget,
		FailureThreshold: req.FailureThreshold,
	})
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(toMonitorResponse(monitor))
}

func (h *MonitorHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projectID := chi.URLParam(r, "projectId")

	monitors, err := h.monitorUsecase.List(r.Context(), usecase.ListMonitorsInput{
		UserID:    userID,
		ProjectID: projectID,
	})
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	data := make([]monitorResponse, 0, len(monitors))
	for _, m := range monitors {
		data = append(data, toMonitorResponse(m))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}

func (h *MonitorHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	monitorID := chi.URLParam(r, "id")

	monitor, err := h.monitorUsecase.GetByID(r.Context(), userID, monitorID)
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toMonitorResponse(monitor))
}

type updateMonitorRequest struct {
	URL              *string `json:"url"`
	IntervalSec      *int    `json:"interval_sec"`
	Channel          *string `json:"channel"`
	ChannelTarget    *string `json:"channel_target"`
	FailureThreshold *int    `json:"failure_threshold"`
}

func (h *MonitorHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	monitorID := chi.URLParam(r, "id")

	var req updateMonitorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body is not valid JSON")
		return
	}

	monitor, err := h.monitorUsecase.Update(r.Context(), usecase.UpdateMonitorInput{
		UserID:           userID,
		MonitorID:        monitorID,
		URL:              req.URL,
		IntervalSec:      req.IntervalSec,
		Channel:          req.Channel,
		ChannelTarget:    req.ChannelTarget,
		FailureThreshold: req.FailureThreshold,
	})
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(toMonitorResponse(monitor))
}

func (h *MonitorHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	monitorID := chi.URLParam(r, "id")

	err := h.monitorUsecase.Delete(r.Context(), usecase.DeleteMonitorInput{
		UserID:    userID,
		MonitorID: monitorID,
	})
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type monitorCheckResponse struct {
	ID         string `json:"id"`
	StatusCode int    `json:"status_code"`
	LatencyMs  int    `json:"latency_ms"`
	IsUp       bool   `json:"is_up"`
	CheckedAt  string `json:"checked_at"`
}

func parseTimeQuery(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (h *MonitorHandler) ListChecks(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	monitorID := chi.URLParam(r, "id")

	from, err := parseTimeQuery(r.URL.Query().Get("from"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "'from' must be RFC3339 format")
		return
	}
	to, err := parseTimeQuery(r.URL.Query().Get("to"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_QUERY", "'to' must be RFC3339 format")
		return
	}

	checks, err := h.monitorUsecase.ListChecks(r.Context(), usecase.ListMonitorChecksInput{
		UserID:    userID,
		MonitorID: monitorID,
		From:      from,
		To:        to,
	})
	if err != nil {
		writeMonitorError(w, h.logger, err)
		return
	}

	data := make([]monitorCheckResponse, 0, len(checks))
	for _, c := range checks {
		data = append(data, monitorCheckResponse{
			ID:         c.ID,
			StatusCode: c.StatusCode,
			LatencyMs:  c.LatencyMs,
			IsUp:       c.IsUp,
			CheckedAt:  c.CheckedAt.UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
}
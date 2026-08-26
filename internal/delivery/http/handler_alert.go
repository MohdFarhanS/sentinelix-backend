package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type AlertRuleHandler struct {
	alertRuleUsecase *usecase.AlertRuleUsecase
}

func NewAlertRuleHandler(alertRuleUsecase *usecase.AlertRuleUsecase) *AlertRuleHandler {
	return &AlertRuleHandler{alertRuleUsecase: alertRuleUsecase}
}

type createAlertRuleRequest struct {
	ConditionType   string `json:"condition_type"`
	Threshold       int    `json:"threshold"`
	WindowMinutes   int    `json:"window_minutes"`
	CooldownMinutes int    `json:"cooldown_minutes"`
	Channel         string `json:"channel"`
	ChannelTarget   string `json:"channel_target"`
}

type alertRuleResponse struct {
	ID              string `json:"id"`
	ProjectID       string `json:"project_id"`
	ConditionType   string `json:"condition_type"`
	Threshold       int    `json:"threshold"`
	WindowMinutes   int    `json:"window_minutes"`
	CooldownMinutes int    `json:"cooldown_minutes"`
	Channel         string `json:"channel"`
	ChannelTarget   string `json:"channel_target"`
	CreatedAt       string `json:"created_at"`
}

func toAlertRuleResponse(r *domain.AlertRule) alertRuleResponse {
	return alertRuleResponse{
		ID:              r.ID,
		ProjectID:       r.ProjectID,
		ConditionType:   r.ConditionType,
		Threshold:       r.Threshold,
		WindowMinutes:   r.WindowMinutes,
		CooldownMinutes: r.CooldownMinutes,
		Channel:         r.Channel,
		ChannelTarget:   r.ChannelTarget,
		CreatedAt:       r.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (h *AlertRuleHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "user not authenticated")
		return
	}

	projectID := chi.URLParam(r, "projectId")

	var req createAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body is not valid JSON")
		return
	}

	// cooldown_minutes default 60 kalau tidak diisi (0) — konsisten
	// dengan DEFAULT 60 di kolom DB, biar behavior sama baik lewat
	// request eksplisit maupun kalau ada insert langsung ke DB.
	if req.CooldownMinutes == 0 {
		req.CooldownMinutes = 60
	}

	rule, err := h.alertRuleUsecase.Create(r.Context(), usecase.CreateAlertRuleInput{
		UserID:          userID,
		ProjectID:       projectID,
		ConditionType:   req.ConditionType,
		Threshold:       req.Threshold,
		WindowMinutes:   req.WindowMinutes,
		CooldownMinutes: req.CooldownMinutes,
		Channel:         req.Channel,
		ChannelTarget:   req.ChannelTarget,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this project")
		case errors.Is(err, domain.ErrAlertConditionTypeInvalid),
			errors.Is(err, domain.ErrAlertChannelInvalid),
			errors.Is(err, domain.ErrAlertThresholdInvalid),
			errors.Is(err, domain.ErrAlertWindowMinutesInvalid),
			errors.Is(err, domain.ErrAlertCooldownMinutesInvalid),
			errors.Is(err, domain.ErrAlertChannelTargetRequired):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create alert rule")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	writeJSON(w, http.StatusCreated, toAlertRuleResponse(rule))
}

func (h *AlertRuleHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projectID := chi.URLParam(r, "projectId")

	rules, err := h.alertRuleUsecase.List(r.Context(), usecase.ListAlertRulesInput{
		UserID:    userID,
		ProjectID: projectID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this project")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch alert rules")
		}
		return
	}

	data := make([]alertRuleResponse, 0, len(rules))
	for _, rule := range rules {
		data = append(data, toAlertRuleResponse(rule))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": data})
}

type updateAlertRuleRequest struct {
	ConditionType   *string `json:"condition_type"`
	Threshold       *int    `json:"threshold"`
	WindowMinutes   *int    `json:"window_minutes"`
	CooldownMinutes *int    `json:"cooldown_minutes"`
	Channel         *string `json:"channel"`
	ChannelTarget   *string `json:"channel_target"`
}

func (h *AlertRuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	ruleID := chi.URLParam(r, "id")

	var req updateAlertRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Request body is not valid JSON")
		return
	}

	rule, err := h.alertRuleUsecase.Update(r.Context(), usecase.UpdateAlertRuleInput{
		UserID:          userID,
		AlertRuleID:     ruleID,
		ConditionType:   req.ConditionType,
		Threshold:       req.Threshold,
		WindowMinutes:   req.WindowMinutes,
		CooldownMinutes: req.CooldownMinutes,
		Channel:         req.Channel,
		ChannelTarget:   req.ChannelTarget,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlertRuleNotFound):
			writeError(w, http.StatusNotFound, "ALERT_RULE_NOT_FOUND", "Alert rule not found")
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this alert rule")
		case errors.Is(err, domain.ErrAlertConditionTypeInvalid),
			errors.Is(err, domain.ErrAlertChannelInvalid),
			errors.Is(err, domain.ErrAlertThresholdInvalid),
			errors.Is(err, domain.ErrAlertThresholdInvalid),
			errors.Is(err, domain.ErrAlertWindowMinutesInvalid),
			errors.Is(err, domain.ErrAlertCooldownMinutesInvalid),
			errors.Is(err, domain.ErrAlertChannelTargetRequired):
			writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed toupdate alert rule")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, toAlertRuleResponse(rule))
}

func (h *AlertRuleHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	ruleID := chi.URLParam(r, "id")

	err := h.alertRuleUsecase.Delete(r.Context(), usecase.DeleteAlertRuleInput{
		UserID:      userID,
		AlertRuleID: ruleID,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrAlertRuleNotFound):
			writeError(w, http.StatusNotFound, "ALERT_RULE_NOT_FOUND", "Alert not found")
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this alert rule")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete alert rule")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

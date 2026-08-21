package http

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type IssueHandler struct {
	issueUsecase *usecase.IssueUsecase
}

func NewIssueHandler(issueUsecase *usecase.IssueUsecase) *IssueHandler {
	return &IssueHandler{issueUsecase: issueUsecase}
}

type issueResponse struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Level    string `json:"level"`
	Status   string `json:"status"`
	Count    int    `json:"count"`
	LastSeen string `json:"last_seen"`
}

func (h *IssueHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projectID := chi.URLParam(r, "projectId")

	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 || limit > 100 {
		limit = 20
	}

	result, err := h.issueUsecase.List(r.Context(), usecase.ListIssuesInput{
		UserID:    userID,
		ProjectID: projectID,
		Status:    r.URL.Query().Get("status"),
		Page:      page,
		Limit:     limit,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this project")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch issue list")
		}
		return
	}

	data := make([]issueResponse, 0, len(result.Issues))
	for _, issue := range result.Issues {
		data = append(data, issueResponse{
			ID:       issue.ID,
			Title:    issue.Title,
			Level:    issue.Level,
			Status:   issue.Status,
			Count:    issue.Count,
			LastSeen: issue.LastSeen.UTC().Format(time.RFC3339),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"data": data,
		"meta": map[string]int{"page": page, "total": result.Total},
	})
}

type issueDetailResponse struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Level     string `json:"level"`
	Status    string `json:"status"`
	Count     int    `json:"count"`
	FirstSeen string `json:"first_seen"`
	LastSeen  string `json:"last_seen"`
}

func (h *IssueHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	issueID := chi.URLParam(r, "id")

	issue, err := h.issueUsecase.GetByID(r.Context(), userID, issueID)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIssueNotFound):
			writeError(w, http.StatusNotFound, "ISSUE_NOT_FOUND", "Issue not found")
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this issue")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch issue detail")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, issueDetailResponse{
		ID:        issue.ID,
		Title:     issue.Title,
		Level:     issue.Level,
		Status:    issue.Status,
		Count:     issue.Count,
		FirstSeen: issue.FirstSeen.UTC().Format(time.RFC3339),
		LastSeen:  issue.LastSeen.UTC().Format(time.RFC3339),
	})
}

type eventResponse struct {
	ID         string         `json:"id"`
	OccurredAt string         `json:"occurred_at"`
	StackTrace string         `json:"stack_trace"`
	Context    map[string]any `json:"context"`
}

func (h *IssueHandler) ListEvents(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	issueID := chi.URLParam(r, "id")

	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit < 1 {
		limit = 50
	}

	events, err := h.issueUsecase.ListEvents(r.Context(), usecase.ListEventsInput{
		UserID:  userID,
		IssueID: issueID,
		Limit:   limit,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrIssueNotFound):
			writeError(w, http.StatusNotFound, "ISSUE_NOT_FOUND", "Issue not found")
		case errors.Is(err, domain.ErrProjectNotFound):
			writeError(w, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		case errors.Is(err, usecase.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "You don't have access to this issue")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch issue events")
		}
		return
	}

	data := make([]eventResponse, 0, len(events))
	for _, event := range events {
		data = append(data, eventResponse{
			ID:         event.ID,
			OccurredAt: event.OccurredAt.UTC().Format(time.RFC3339),
			StackTrace: event.StackTrace,
			Context:    event.Context,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": data})
}
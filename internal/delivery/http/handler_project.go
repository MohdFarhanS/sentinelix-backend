package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type ProjectHandler struct {
	projectUsecase *usecase.ProjectUsecase
}

func NewProjectHandler(projectUsecase *usecase.ProjectUsecase) *ProjectHandler {
	return &ProjectHandler{projectUsecase: projectUsecase}
}

type createProjectRequest struct {
	Name string `json:"name"`
}

func (h *ProjectHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Project name is required")
		return
	}

	out, err := h.projectUsecase.Create(r.Context(), usecase.CreateProjectInput{
		UserID: userID,
		Name:   req.Name,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to create project")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"id":      out.ID,
		"name":    out.Name,
		"slug":    out.Slug,
		"api_key": out.APIKey,
	})
}

func (h *ProjectHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projects, err := h.projectUsecase.List(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to fetch project list")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{"data": projects})
}

// Delete — response 204 No Content, konsisten sama DELETE /alert-rules/:id
// & DELETE /monitors/:id (04-API-DESIGN.md §6, §7).
func (h *ProjectHandler) Delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := UserIDFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "User not authenticated")
		return
	}

	projectID := chi.URLParam(r, "id")

	err := h.projectUsecase.Delete(r.Context(), userID, projectID)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, domain.ErrProjectNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Project not found")
	case errors.Is(err, usecase.ErrForbidden):
		writeError(w, http.StatusForbidden, "FORBIDDEN", "You do not own this project")
	default:
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete project")
	}
}
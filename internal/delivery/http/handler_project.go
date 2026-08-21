package http

import (
	"encoding/json"
	"net/http"

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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": projects})
}
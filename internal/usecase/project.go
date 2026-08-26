package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/rs/zerolog"
)

type ProjectUsecase struct {
	projectRepo  domain.ProjectRepository
	auditLogRepo domain.AuditLogRepository
	logger       zerolog.Logger
}

func NewProjectUsecase(projectRepo domain.ProjectRepository, auditLogRepo domain.AuditLogRepository, logger zerolog.Logger) *ProjectUsecase {
	return &ProjectUsecase{projectRepo: projectRepo, auditLogRepo: auditLogRepo, logger: logger}
}

// writeAuditLog — dipusatkan di sini karena DIPAKAI DUA KALI di dalam
// struct yang SAMA (Create & Delete) — beda dari kasus AlertRuleUsecase
// yang punya method sendiri; itu bukan duplikasi lintas-tipe, ini
// duplikasi ASLI di dalam satu tipe yang sama, jadi extract di sini valid,
// bukan premature abstraction.
func (uc *ProjectUsecase) writeAuditLog(ctx context.Context, actorUserID, action, resourceID string, metadata map[string]any) {
	if err := uc.auditLogRepo.Create(ctx, &domain.AuditLog{
		ActorUserID:  &actorUserID,
		Action:       action,
		ResourceType: domain.ResourceTypeProject,
		ResourceID:   resourceID,
		Metadata:     metadata,
	}); err != nil {
		uc.logger.Error().
			Err(err).
			Str("action", action).
			Str("resource_id", resourceID).
			Str("actor_user_id", actorUserID).
			Msg("failed to write audit log")
	}
}

type CreateProjectInput struct {
	UserID string
	Name   string
}

type CreateProjectOutput struct {
	ID     string
	Name   string
	Slug   string
	APIKey string
}

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = slugSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "project"
	}
	return s
}

const APIKeyPrefix = "si_live_"

func generateAPIKey() (plaintext, hash string, err error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	plaintext = APIKeyPrefix + hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
}

func randomSlugSuffix() string {
	raw := make([]byte, 3)
	_, _ = rand.Read(raw)
	return hex.EncodeToString(raw)
}

func (uc *ProjectUsecase) Create(ctx context.Context, in CreateProjectInput) (*CreateProjectOutput, error) {
	apiKey, apiKeyHash, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	p := &domain.Project{
		UserID:     in.UserID,
		Name:       in.Name,
		Slug:       slugify(in.Name) + "-" + randomSlugSuffix(),
		APIKeyHash: apiKeyHash,
	}

	if err := uc.projectRepo.Create(ctx, p); err != nil {
		return nil, err
	}

	uc.writeAuditLog(ctx, in.UserID, domain.ActionProjectAPIKeyCreated, p.ID, map[string]any{
		"project_name": p.Name,
	})

	return &CreateProjectOutput{ID: p.ID, Name: p.Name, Slug: p.Slug, APIKey: apiKey}, nil
}

type ProjectSummary struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

func (uc *ProjectUsecase) List(ctx context.Context, userID string) ([]ProjectSummary, error) {
	projects, err := uc.projectRepo.ListByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	summaries := make([]ProjectSummary, 0, len(projects))
	for _, p := range projects {
		summaries = append(summaries, ProjectSummary{ID: p.ID, Name: p.Name, Slug: p.Slug})
	}
	return summaries, nil
}

func (uc *ProjectUsecase) VerifyOwnership(ctx context.Context, userID, projectID string) error {
	project, err := uc.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return err
	}
	if project.UserID != userID {
		return ErrForbidden
	}
	return nil
}

// Delete — ownership check dulu (pola sama seperti VerifyOwnership /
// getOwnedAlertRule), baru hapus. Metadata audit log nyimpen nama project
// (bukan API key/hash-nya — konsisten sama Create) SEBELUM dihapus, karena
// setelah ini project-nya sudah tidak ada buat dicek lagi.
func (uc *ProjectUsecase) Delete(ctx context.Context, userID, projectID string) error {
	project, err := uc.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return err
	}
	if project.UserID != userID {
		return ErrForbidden
	}

	if err := uc.projectRepo.Delete(ctx, projectID); err != nil {
		return err
	}

	uc.writeAuditLog(ctx, userID, domain.ActionProjectDeleted, project.ID, map[string]any{
		"project_name": project.Name,
	})

	return nil
}
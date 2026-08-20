package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type ProjectUsecase struct {
	projectRepo domain.ProjectRepository
}

func NewProjectUsecase(projectRepo domain.ProjectRepository) *ProjectUsecase {
	return &ProjectUsecase{projectRepo: projectRepo}
}

type CreateProjectInput struct {
	UserID string
	Name   string
}

type CreateProjectOutput struct {
	ID     string
	Name   string
	Slug   string
	APIKey string // plaintext — hanya dikembalikan sekali di sini
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

// APIKeyPrefix di-export supaya bisa dirujuk dari test tanpa duplikasi
// string literal — satu sumber kebenaran, tidak perlu ubah 2 tempat kalau
// prefix-nya ganti lagi nanti.
const APIKeyPrefix = "si_live_"

// generateAPIKey bikin API key plaintext format si_live_<32 hex char>
// (sesuai contoh di 04-API-DESIGN.md §3) + hash SHA-256-nya buat disimpan
// di kolom api_key_hash — konsisten dengan cara lookup di ingest (Sprint 2).
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

// randomSlugSuffix nambah entropi kecil ke slug supaya risiko bentrok di
// UNIQUE(slug) rendah, tanpa retry-loop query DB (YAGNI — kalau nanti
// collision beneran kejadian di practice, baru kita tambah retry).
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

// VerifyOwnership mengecek project exist DAN dimiliki oleh userID. Dipakai
// di tempat yang cuma butuh cek akses tanpa perlu data project lengkap
// (misal: WS handshake sebelum upgrade connection) — pola ownership-check
// yang sama seperti IssueUsecase.getOwnedIssue, cuma untuk project secara
// langsung (bukan lewat issue).
func (uc *ProjectUsecase) VerifyOwnership(ctx context.Context, userID, projectID string) error {
	project, err := uc.projectRepo.GetByID(ctx, projectID)
	if err != nil {
		return err // termasuk domain.ErrProjectNotFound
	}
	if project.UserID != userID {
		return ErrForbidden
	}
	return nil
}
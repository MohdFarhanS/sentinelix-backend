package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

// mockProjectRepo struct-nya sudah didefinisikan di ingest_event_test.go
// (embed mock.Mock). mockAuditLogRepo sudah didefinisikan di
// alert_rule_test.go — satu package usecase_test, TIDAK boleh redeclare.

func (m *mockProjectRepo) Create(ctx context.Context, p *domain.Project) error {
	args := m.Called(ctx, p)
	p.ID = "generated-id"
	return args.Error(0)
}

func (m *mockProjectRepo) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Project), args.Error(1)
}

func (m *mockProjectRepo) ListByUserID(ctx context.Context, userID string) ([]*domain.Project, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*domain.Project), args.Error(1)
}

// NEW: wajib ada — domain.ProjectRepository baru nambah method Delete,
// mock ini harus ikut implementasi supaya masih valid sebagai
// domain.ProjectRepository di semua test (termasuk yang lama, yang tidak
// pernah manggil Delete sekalipun).
func (m *mockProjectRepo) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func TestProjectUsecase_Create(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Project")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).Return(nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	out, err := uc.Create(context.Background(), usecase.CreateProjectInput{
		UserID: "user-1",
		Name:   "NewsPortal Prod",
	})

	assert.NoError(t, err)
	assert.Equal(t, "generated-id", out.ID)
	assert.True(t, strings.HasPrefix(out.Slug, "newsportal-prod-"))
	assert.True(t, strings.HasPrefix(out.APIKey, usecase.APIKeyPrefix))
	assert.Len(t, out.APIKey, len(usecase.APIKeyPrefix)+32)
}

func TestProjectUsecase_Create_WritesAuditLog_WithoutCredential(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Project")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.MatchedBy(func(log *domain.AuditLog) bool {
		if log.Action != domain.ActionProjectAPIKeyCreated || log.ResourceID != "generated-id" {
			return false
		}
		for _, v := range log.Metadata {
			if s, ok := v.(string); ok && strings.HasPrefix(s, usecase.APIKeyPrefix) {
				return false
			}
		}
		return true
	})).Return(nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	_, err := uc.Create(context.Background(), usecase.CreateProjectInput{
		UserID: "user-1",
		Name:   "NewsPortal Prod",
	})

	assert.NoError(t, err)
	auditLogRepo.AssertExpectations(t)
}

func TestProjectUsecase_Create_AuditLogFailure_StillSucceeds(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Project")).Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).
		Return(errAuditLogWrite)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	out, err := uc.Create(context.Background(), usecase.CreateProjectInput{
		UserID: "user-1",
		Name:   "NewsPortal Prod",
	})

	assert.NoError(t, err)
	assert.Equal(t, "generated-id", out.ID)
}

func TestProjectUsecase_List(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("ListByUserID", mock.Anything, "user-1").Return([]*domain.Project{
		{ID: "p1", Name: "Project A", Slug: "project-a-abc123"},
	}, nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	out, err := uc.List(context.Background(), "user-1")

	assert.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Equal(t, "Project A", out[0].Name)
}

func TestProjectUsecase_List_Empty(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("ListByUserID", mock.Anything, "user-2").Return([]*domain.Project{}, nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	out, err := uc.List(context.Background(), "user-2")

	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestProjectUsecase_VerifyOwnership_Success(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-1")

	assert.NoError(t, err)
}

func TestProjectUsecase_VerifyOwnership_NotFound(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-x")

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
}

func TestProjectUsecase_VerifyOwnership_Forbidden(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)
	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-1")

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}

// NEW: ProjectUsecase.Delete — ownership check, hapus, audit log.

func TestProjectUsecase_Delete_Success(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1", Name: "NewsPortal Prod",
	}, nil)
	repo.On("Delete", mock.Anything, "proj-1").Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.MatchedBy(func(log *domain.AuditLog) bool {
		return log.Action == domain.ActionProjectDeleted &&
			log.ResourceID == "proj-1" &&
			*log.ActorUserID == "user-1" &&
			log.Metadata["project_name"] == "NewsPortal Prod"
	})).Return(nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), "user-1", "proj-1")

	assert.NoError(t, err)
	repo.AssertCalled(t, "Delete", mock.Anything, "proj-1")
	auditLogRepo.AssertExpectations(t)
}

func TestProjectUsecase_Delete_NotFound(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	repo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), "user-1", "proj-x")

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestProjectUsecase_Delete_Forbidden_DoesNotDelete — ownership check
// GAGAL (bukan pemilik) harus mencegah Delete() di repo bahkan
// TERPANGGIL — bukan cuma return error setelah kadung menghapus.
func TestProjectUsecase_Delete_Forbidden_DoesNotDelete(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), "user-1", "proj-1")

	assert.ErrorIs(t, err, usecase.ErrForbidden)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

// TestProjectUsecase_Delete_AuditLogFailure_StillSucceeds — best-effort,
// sama seperti Create: audit log gagal tulis TIDAK BOLEH menggagalkan
// Delete yang sudah sukses.
func TestProjectUsecase_Delete_AuditLogFailure_StillSucceeds(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1", Name: "NewsPortal Prod",
	}, nil)
	repo.On("Delete", mock.Anything, "proj-1").Return(nil)
	auditLogRepo.On("Create", mock.Anything, mock.Anything).Return(errAuditLogWrite)

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), "user-1", "proj-1")

	assert.NoError(t, err)
}

func TestProjectUsecase_Delete_RepoErrorPropagates(t *testing.T) {
	repo := new(mockProjectRepo)
	auditLogRepo := new(mockAuditLogRepo)

	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)
	repo.On("Delete", mock.Anything, "proj-1").Return(errAuditLogWrite) // reuse error, bukan berarti terkait audit log

	uc := usecase.NewProjectUsecase(repo, auditLogRepo, zerolog.Nop())
	err := uc.Delete(context.Background(), "user-1", "proj-1")

	assert.Error(t, err)
	// Delete di repo GAGAL — audit log TIDAK BOLEH ditulis (project belum
	// beneran hilang, jangan catat "deleted" padahal masih ada).
	auditLogRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}
package usecase_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

// mockProjectRepo struct-nya sudah didefinisikan di ingest_event_test.go
// (embed mock.Mock). Karena satu package usecase_test, method tambahan
// ini nyambung ke struct yang sama — TIDAK boleh redeclare `type mockProjectRepo struct`.

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

func TestProjectUsecase_Create(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("Create", mock.Anything, mock.AnythingOfType("*domain.Project")).Return(nil)

	uc := usecase.NewProjectUsecase(repo)
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

func TestProjectUsecase_List(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("ListByUserID", mock.Anything, "user-1").Return([]*domain.Project{
		{ID: "p1", Name: "Project A", Slug: "project-a-abc123"},
	}, nil)

	uc := usecase.NewProjectUsecase(repo)
	out, err := uc.List(context.Background(), "user-1")

	assert.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Equal(t, "Project A", out[0].Name)
}

func TestProjectUsecase_List_Empty(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("ListByUserID", mock.Anything, "user-2").Return([]*domain.Project{}, nil)

	uc := usecase.NewProjectUsecase(repo)
	out, err := uc.List(context.Background(), "user-2")

	assert.NoError(t, err)
	assert.Empty(t, out)
}

func TestProjectUsecase_VerifyOwnership_Success(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-1",
	}, nil)

	uc := usecase.NewProjectUsecase(repo)
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-1")

	assert.NoError(t, err)
}

func TestProjectUsecase_VerifyOwnership_NotFound(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("GetByID", mock.Anything, "proj-x").Return(nil, domain.ErrProjectNotFound)

	uc :=usecase.NewProjectUsecase(repo)
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-x")

	assert.ErrorIs(t, err, domain.ErrProjectNotFound)
}

func TestProjectUsecase_VerifyOwnership_Forbidden(t *testing.T) {
	repo := new(mockProjectRepo)
	repo.On("GetByID", mock.Anything, "proj-1").Return(&domain.Project{
		ID: "proj-1", UserID: "user-OTHER",
	}, nil)

	uc := usecase.NewProjectUsecase(repo)
	err := uc.VerifyOwnership(context.Background(), "user-1", "proj-1")

	assert.ErrorIs(t, err, usecase.ErrForbidden)
}
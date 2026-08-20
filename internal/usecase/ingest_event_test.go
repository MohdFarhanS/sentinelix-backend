package usecase_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

type mockProjectRepo struct{ mock.Mock }

func (m *mockProjectRepo) GetByAPIKeyHash(ctx context.Context, hash string) (*domain.Project, error) {
	args := m.Called(ctx, hash)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*domain.Project), args.Error(1)
}

type mockRateLimiter struct{ mock.Mock }

func (m *mockRateLimiter) Allow(ctx context.Context, key string) (bool, error) {
	args := m.Called(ctx, key)
	return args.Bool(0), args.Error(1)
}

type mockQueue struct{ mock.Mock }

func (m *mockQueue) Push(ctx context.Context, event *domain.Event) error {
	args := m.Called(ctx, event)
	return args.Error(0)
}

func TestIngestEvent_Success(t *testing.T) {
	repo := new(mockProjectRepo)
	limiter := new(mockRateLimiter)
	queue := new(mockQueue)

	repo.On("GetByAPIKeyHash", mock.Anything, mock.Anything).
		Return(&domain.Project{ID: "project-1"}, nil)
	limiter.On("Allow", mock.Anything, mock.Anything).Return(true, nil)
	queue.On("Push", mock.Anything, mock.Anything).Return(nil)

	uc := usecase.NewIngestEventUsecase(repo, limiter, queue)
	err := uc.Execute(context.Background(), usecase.IngestEventInput{
		RawAPIKey: "si_live_valid",
		Message:   "TypeError: something broke",
	})

	assert.NoError(t, err)
	queue.AssertCalled(t, "Push", mock.Anything, mock.MatchedBy(func(e *domain.Event) bool {
		return e.ProjectID == "project-1" && e.Level == "error"
	}))
}

func TestIngestEvent_RateLimited(t *testing.T) {
	repo := new(mockProjectRepo)
	limiter := new(mockRateLimiter)
	queue := new(mockQueue)

	limiter.On("Allow", mock.Anything, mock.Anything).Return(false, nil)

	uc := usecase.NewIngestEventUsecase(repo, limiter, queue)
	err := uc.Execute(context.Background(), usecase.IngestEventInput{
		RawAPIKey: "si_live_spammy",
		Message:   "some error",
	})

	assert.ErrorIs(t, err, usecase.ErrRateLimited)
	repo.AssertNotCalled(t, "GetByAPIKeyHash")
	queue.AssertNotCalled(t, "Push")
}

func TestIngestEvent_InvalidAPIKey(t *testing.T) {
	repo := new(mockProjectRepo)
	limiter := new(mockRateLimiter)
	queue := new(mockQueue)

	limiter.On("Allow", mock.Anything, mock.Anything).Return(true, nil)
	repo.On("GetByAPIKeyHash", mock.Anything, mock.Anything).
		Return(nil, domain.ErrProjectNotFound)

	uc := usecase.NewIngestEventUsecase(repo, limiter, queue)
	err := uc.Execute(context.Background(), usecase.IngestEventInput{
		RawAPIKey: "si_live_wrong",
		Message:   "some error",
	})

	assert.ErrorIs(t, err, usecase.ErrInvalidAPIKey)
	queue.AssertNotCalled(t, "Push")
}

func TestIngestEvent_ValidationFails(t *testing.T) {
	repo := new(mockProjectRepo)
	limiter := new(mockRateLimiter)
	queue := new(mockQueue)

	limiter.On("Allow", mock.Anything, mock.Anything).Return(true, nil)
	repo.On("GetByAPIKeyHash", mock.Anything, mock.Anything).
		Return(&domain.Project{ID: "project-1"}, nil)

	uc := usecase.NewIngestEventUsecase(repo, limiter, queue)
	err := uc.Execute(context.Background(), usecase.IngestEventInput{
		RawAPIKey: "si_live_valid",
		Message:   "",
	})

	assert.ErrorIs(t, err, domain.ErrEventMessageRequired)
	queue.AssertNotCalled(t, "Push")
}

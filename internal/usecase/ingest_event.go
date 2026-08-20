package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/apikey"
)

var (
	ErrInvalidAPIKey = errors.New("invalid api key")
	ErrRateLimited   = errors.New("rate limit exceeded")
)

type IngestEventInput struct {
	RawAPIKey  string
	Level      string
	Message    string
	StackTrace string
	Context    map[string]any
}

type IngestEventUsecase struct {
	projectRepo domain.ProjectRepository
	rateLimiter domain.RateLimiter
	queue       domain.EventQueue
}

func NewIngestEventUsecase(
	projectRepo domain.ProjectRepository,
	rateLimiter domain.RateLimiter,
	queue domain.EventQueue,
) *IngestEventUsecase {
	return &IngestEventUsecase{projectRepo: projectRepo, rateLimiter: rateLimiter, queue: queue}
}

func (uc *IngestEventUsecase) Execute(ctx context.Context, input IngestEventInput) error {
	hashedKey := apikey.Hash(input.RawAPIKey)

	allowed, err := uc.rateLimiter.Allow(ctx, hashedKey)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrRateLimited
	}

	project, err := uc.projectRepo.GetByAPIKeyHash(ctx, hashedKey)
	if err != nil {
		if errors.Is(err, domain.ErrProjectNotFound) {
			return ErrInvalidAPIKey
		}
		return err
	}

	event := &domain.Event{
		ProjectID:  project.ID,
		Level:      input.Level,
		Message:    input.Message,
		StackTrace: input.StackTrace,
		Context:    input.Context,
		OccurredAt: time.Now().UTC(),
	}
	if err := event.Validate(); err != nil {
		return err
	}

	return uc.queue.Push(ctx, event)
}
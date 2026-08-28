package usecase

import (
	"context"
	"errors"
	"time"

	"github.com/rs/zerolog"

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
	logger      zerolog.Logger
}

func NewIngestEventUsecase(
	projectRepo domain.ProjectRepository,
	rateLimiter domain.RateLimiter,
	queue domain.EventQueue,
	logger zerolog.Logger,
) *IngestEventUsecase {
	return &IngestEventUsecase{projectRepo: projectRepo, rateLimiter: rateLimiter, queue: queue, logger: logger}
}

func (uc *IngestEventUsecase) Execute(ctx context.Context, input IngestEventInput) error {
	hashedKey := apikey.Hash(input.RawAPIKey)

	allowed, err := uc.rateLimiter.Allow(ctx, hashedKey)
	if err != nil {
		// Fail-open, SENGAJA: kalau Redis SENDIRI yang error (bukan limit
		// beneran kelampauan), lebih baik terima event daripada menolak
		// laporan error asli gara-gara infra rate-limiter bermasalah —
		// kehilangan sinyal error production jauh lebih mahal daripada
		// kelonggaran sesaat di rate limit ingest.
		//
		// Ini TIDAK membuka abuse tanpa syarat: request dengan API key
		// tidak valid tetap ditolak di GetByAPIKeyHash di bawah, terlepas
		// dari hasil rate limit. Fail-open cuma menghilangkan throttling
		// SEMENTARA selama Redis bermasalah, bukan menghilangkan validasi.
		//
		// Log level Error (bukan Warn) SENGAJA — ini sinyal infra Redis
		// bermasalah, harus mencolok, bukan tenggelam di log info biasa.
		uc.logger.Error().Err(err).Msg("rate limiter error, failing open (event still accepted)")
		allowed = true
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
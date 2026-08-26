package worker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

const (
	ingestStreamKey = "queue:ingest"
	consumerGroup   = "ingest-workers"
	consumerName    = "worker-1"
)

type IngestConsumer struct {
	client        *redis.Client
	logger        zerolog.Logger
	groupIssue    *usecase.GroupIssueUsecase
	broadcaster   *redisrepo.Broadcaster
	evaluateAlert *usecase.EvaluateAlertUsecase
}

func NewIngestConsumer(
	client *redis.Client,
	logger zerolog.Logger,
	groupIssue *usecase.GroupIssueUsecase,
	broadcaster *redisrepo.Broadcaster,
	evaluateAlert *usecase.EvaluateAlertUsecase,
) *IngestConsumer {
	return &IngestConsumer{
		client: client, logger: logger, groupIssue: groupIssue,
		broadcaster: broadcaster, evaluateAlert: evaluateAlert}
}

func (c *IngestConsumer) ensureGroup(ctx context.Context) error {
	err := c.client.XGroupCreateMkStream(ctx, ingestStreamKey, consumerGroup, "0").Err()
	if err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		return err
	}
	return nil
}

func (c *IngestConsumer) Run(ctx context.Context) error {
	if err := c.ensureGroup(ctx); err != nil {
		return err
	}
	c.logger.Info().Msg("ingest consumer started, waiting for events...")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		streams, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: consumerName,
			Streams:  []string{ingestStreamKey, ">"},
			Count:    10,
			Block:    5 * time.Second,
		}).Result()

		if err != nil {
			if err == redis.Nil {
				continue
			}
			c.logger.Error().Err(err).Msg("failed to read from stream")
			continue
		}

		for _, stream := range streams {
			for _, msg := range stream.Messages {
				c.processMessage(ctx, msg)
			}
		}
	}
}

func (c *IngestConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	// NOTE: pola ack-selalu (ack di akhir apapun hasilnya) dipertahankan
	// sama seperti versi Sprint 2 — retry/DLQ (NFR-3) belum di-scope di
	// Sprint 3 ini, akan ditangani terpisah nanti.
	defer func() {
		if err := c.client.XAck(ctx, ingestStreamKey, consumerGroup, msg.ID).Err(); err != nil {
			c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to ack message")
		}
	}()

	raw, ok := msg.Values["data"].(string)
	if !ok {
		c.logger.Error().Str("message_id", msg.ID).Msg("data field missing or not a string")
		return
	}

	var event domain.Event
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to unmarshal event")
		return
	}

	issue, wasCreated, err := c.groupIssue.Execute(ctx, &event)
	if err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to group issue")
		return
	}

	eventType, data := buildBroadcastPayload(issue, wasCreated)
	if err := c.broadcaster.Publish(ctx, issue.ProjectID, eventType, data); err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to publish broadcast message")
	}

	// Evaluasi alert rule "new_issue" HANYA kalau issue ini benar-benar
	// baru (wasCreated) — issue lama yang cuma nambah count bukan
	// kondisi "new_issue". Dijalankan setelah broadcast, bukan sebelum,
	// biar dashboard tetap update real-time walau pengiriman notifikasi
	// gagal/lambat (dua concern independen, gagal salah satu tidak
	// menghambat yang lain).
	if wasCreated {
		if err := c.evaluateAlert.EvaluateNewIssue(ctx, issue); err != nil {
			c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to evaluate new_issue alert rules")
		}
	}

	c.logger.Info().
		Str("message_id", msg.ID).
		Str("issue_id", issue.ID).
		Bool("was_created", wasCreated).
		Int("count", issue.Count).
		Msg("event grouped successfully")
}

type issueCreatedPayload struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type issueUpdatedPayload struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// buildBroadcastPayload nentuin type & shape data sesuai format event yang
// sudah didefinisikan di 04-API-DESIGN.md §8.
func buildBroadcastPayload(issue *domain.Issue, wasCreated bool) (string, interface{}) {
	if wasCreated {
		return "issue.created", issueCreatedPayload{ID: issue.ID, Title: issue.Title}
	}
	return "issue.updated", issueUpdatedPayload{ID: issue.ID, Count: issue.Count}
}

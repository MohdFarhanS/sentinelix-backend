package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

const (
	ingestStreamKey = "queue:ingest"
	// dlqStreamKey — Redis Stream terpisah buat event yang gagal diproses
	// setelah melebihi maxDeliveryCount. Dipilih Stream (bukan List/Set)
	// biar konsisten tooling-nya dengan queue:ingest (bisa di-XRange,
	// di-inspect pakai redis-cli/RedisInsight yang sama).
	dlqStreamKey  = "queue:ingest:dlq"
	consumerGroup = "ingest-workers"
	consumerName  = "worker-1"
)

type IngestConsumer struct {
	client        *redis.Client
	logger        zerolog.Logger
	groupIssue    *usecase.GroupIssueUsecase
	broadcaster   *redisrepo.Broadcaster
	evaluateAlert *usecase.EvaluateAlertUsecase

	// maxDeliveryCount, reclaimMinIdle, reclaimInterval — parameter NFR-3
	// (retry + DLQ), SENGAJA jadi field (bukan konstanta package) supaya
	// unit test bisa pakai nilai kecil (reclaim cepat, dalam hitungan
	// milidetik) tanpa perlu menunggu nilai produksi yang sengaja besar.
	maxDeliveryCount int64
	reclaimMinIdle   time.Duration
	reclaimInterval  time.Duration
}

func NewIngestConsumer(
	client *redis.Client,
	logger zerolog.Logger,
	groupIssue *usecase.GroupIssueUsecase,
	broadcaster *redisrepo.Broadcaster,
	evaluateAlert *usecase.EvaluateAlertUsecase,
	maxDeliveryCount int64,
	reclaimMinIdle time.Duration,
	reclaimInterval time.Duration,
) *IngestConsumer {
	return &IngestConsumer{
		client: client, logger: logger, groupIssue: groupIssue,
		broadcaster: broadcaster, evaluateAlert: evaluateAlert,
		maxDeliveryCount: maxDeliveryCount,
		reclaimMinIdle:   reclaimMinIdle,
		reclaimInterval:  reclaimInterval,
	}
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

// RunReclaim — loop terpisah (goroutine sendiri, dijalankan bareng Run()
// di cmd/worker/main.go) yang secara berkala scan pesan PENDING (sudah
// di-XReadGroup tapi belum di-ack) yang idle lebih lama dari
// reclaimMinIdle — tanda percobaan sebelumnya gagal (bukan lagi diproses
// aktif, karena kalau masih aktif idle-nya pasti pendek). Redis Streams
// sendiri yang nge-track delivery count per pesan lewat XPendingExt —
// tidak perlu counter manual di kode kita (NFR-3: retry + DLQ).
func (c *IngestConsumer) RunReclaim(ctx context.Context) error {
	ticker := time.NewTicker(c.reclaimInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.reclaimOnce(ctx)
		}
	}
}

func (c *IngestConsumer) reclaimOnce(ctx context.Context) {
	pending, err := c.client.XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: ingestStreamKey,
		Group:  consumerGroup,
		Idle:   c.reclaimMinIdle,
		Start:  "-",
		End:    "+",
		Count:  50,
	}).Result()
	if err != nil {
		c.logger.Error().Err(err).Msg("failed to read pending messages for reclaim")
		return
	}

	for _, p := range pending {
		claimed, err := c.client.XClaim(ctx, &redis.XClaimArgs{
			Stream:   ingestStreamKey,
			Group:    consumerGroup,
			Consumer: consumerName,
			MinIdle:  c.reclaimMinIdle,
			Messages: []string{p.ID},
		}).Result()
		if err != nil || len(claimed) == 0 {
			if err != nil {
				c.logger.Error().Err(err).Str("message_id", p.ID).Msg("failed to claim pending message")
			}
			continue
		}
		msg := claimed[0]

		if p.RetryCount > c.maxDeliveryCount {
			c.logger.Error().Str("message_id", p.ID).Int64("delivery_count", p.RetryCount).
				Msg("max retries exceeded, moving to DLQ")
			c.moveToDLQ(ctx, msg, fmt.Sprintf("exceeded max retries (%d)", c.maxDeliveryCount))
			continue
		}

		c.logger.Info().Str("message_id", msg.ID).Int64("attempt", p.RetryCount+1).
			Msg("retrying previously failed message")
		c.processMessage(ctx, msg)
	}
}

// moveToDLQ — pindahkan pesan (payload asli + alasan gagal) ke stream
// terpisah, lalu ack dari stream utama. Best-effort di dua langkahnya:
// kegagalan XAdd/XAck di sini di-log, TIDAK di-retry lagi (kalau sampai
// gagal di titik ini, kemungkinan besar masalahnya di Redis itu sendiri,
// bukan di pesan spesifik ini).
func (c *IngestConsumer) moveToDLQ(ctx context.Context, msg redis.XMessage, reason string) {
	dlqPayload := map[string]interface{}{
		"original_id": msg.ID,
		"data":        msg.Values["data"],
		"reason":      reason,
		"failed_at":   time.Now().UTC().Format(time.RFC3339),
	}
	raw, _ := json.Marshal(dlqPayload)

	if err := c.client.XAdd(ctx, &redis.XAddArgs{
		Stream: dlqStreamKey,
		Values: map[string]interface{}{"data": string(raw)},
	}).Err(); err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to move message to DLQ")
	}

	if err := c.client.XAck(ctx, ingestStreamKey, consumerGroup, msg.ID).Err(); err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to ack message after moving to DLQ")
	}
}

func (c *IngestConsumer) processMessage(ctx context.Context, msg redis.XMessage) {
	raw, ok := msg.Values["data"].(string)
	if !ok {
		// Pesan cacat secara struktural (bukan masalah infra/transient) —
		// retry TIDAK akan pernah berhasil, langsung DLQ tanpa nunggu
		// reclaim loop.
		c.logger.Error().Str("message_id", msg.ID).Msg("data field missing or not a string, moving to DLQ")
		c.moveToDLQ(ctx, msg, "data field missing or not a string")
		return
	}

	var event domain.Event
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		// Sama alasannya seperti di atas — JSON invalid tidak akan
		// pernah valid cuma dengan dicoba ulang.
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to unmarshal event, moving to DLQ")
		c.moveToDLQ(ctx, msg, "unmarshal error: "+err.Error())
		return
	}

	issue, wasCreated, err := c.groupIssue.Execute(ctx, &event)
	if err != nil {
		// Kandidat kuat kegagalan TRANSIENT (Postgres/Neon tidak bisa
		// diakses, dsb) — SENGAJA TIDAK di-ack di sini. Pesan tetap
		// "pending" di consumer group, ditemukan lagi oleh reclaimOnce
		// setelah idle > reclaimMinIdle, di-retry sampai maxDeliveryCount
		// kali sebelum akhirnya masuk DLQ.
		c.logger.Error().Err(err).Str("message_id", msg.ID).
			Msg("failed to group issue, leaving pending for retry")
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

	// Ack CUMA di titik SUKSES ini -- groupIssue.Execute berhasil berarti
	// data sudah aman tersimpan di Postgres. Kegagalan broadcast/alert di
	// atas TIDAK membatalkan ack ini (concern terpisah, sudah di-log,
	// issue-nya sendiri tetap tersimpan benar).
	if err := c.client.XAck(ctx, ingestStreamKey, consumerGroup, msg.ID).Err(); err != nil {
		c.logger.Error().Err(err).Str("message_id", msg.ID).Msg("failed to ack message")
	}
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
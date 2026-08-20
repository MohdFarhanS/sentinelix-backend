package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

const ingestStreamKey = "queue:ingest"

type EventQueue struct {
	client *redis.Client
}

func NewEventQueue(client *redis.Client) *EventQueue {
	return &EventQueue{client: client}
}

func (q *EventQueue) Push(ctx context.Context, event *domain.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	err = q.client.XAdd(ctx, &redis.XAddArgs{
		Stream: ingestStreamKey,
		Values: map[string]interface{}{"data": payload},
	}).Err()
	if err != nil {
		return fmt.Errorf("xadd: %w", err)
	}
	return nil
}
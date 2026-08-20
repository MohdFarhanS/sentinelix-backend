package redis

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/redis/go-redis/v9"
)

const wsBroadcastChannel = "ws:broadcast"

// BroadcastMessage adalah envelope yang lewat Redis Pub/Sub. ProjectID
// dipakai API buat routing ke room yang tepat di hub — TIDAK diteruskan ke
// client. Type & Data itu yang diteruskan apa adanya ke WebSocket client,
// formatnya sesuai 04-API-DESIGN.md §8.
type BroadcastMessage struct {
	ProjectID string      `json:"project_id"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
}

type Broadcaster struct {
	client *redis.Client
}

func NewBroadcaster(client *redis.Client) *Broadcaster {
	return &Broadcaster{client: client}
}

// Publish dipanggil dari proses worker, setelah GroupIssueUsecase.Execute
// sukses.
func (b *Broadcaster) Publish(ctx context.Context, projectID, eventType string, data interface{}) error {
	msg := BroadcastMessage{ProjectID: projectID, Type: eventType, Data: data}
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal broadcast message: %w", err)
	}
	if err := b.client.Publish(ctx, wsBroadcastChannel, payload).Err(); err != nil {
		return fmt.Errorf("publish: %w", err)
	}
	return nil
}

// Subscribe dipanggil sekali dari proses API saat startup. Caller (hub
// subscriber goroutine) yang baca dari channel-nya terus-menerus.
func (b *Broadcaster) Subscribe(ctx context.Context) *redis.PubSub {
	return b.client.Subscribe(ctx, wsBroadcastChannel)
}
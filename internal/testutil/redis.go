package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// NewRedisClient — sama filosofi seperti NewPostgresPool: container baru
// per pemanggilan, isolasi penuh antar test.
func NewRedisClient(t *testing.T) *redis.Client {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		t.Fatalf("failed to start redis container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate redis container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx)
	if err != nil {
		t.Fatalf("failed to get connection string: %V", err)
	}

	opt, err := redis.ParseURL(connStr)
	if err != nil {
		t.Fatalf("failed to parse redis connection string: %v", err)
	}

	client := redis.NewClient(opt)
	t.Cleanup(func() { _ = client.Close() })

	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := client.Ping(ctxPing).Err(); err != nil {
		t.Fatalf("failed to ping redis container: %v", err)
	}

	return client
}
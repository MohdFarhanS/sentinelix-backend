package worker_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/testutil"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/MohdFarhanS/sentinelix-backend/internal/worker"
)

// noopNotifier -- fake Notifier, dipakai supaya EvaluateAlertUsecase bisa
// di-construct dengan Postgres asli tanpa perlu Resend/Slack API key
// beneran di test. Tidak ada alert rule yang dibuat di test-test di bawah,
// jadi Notify() TIDAK PERNAH benar-benar dipanggil -- ini cuma buat
// memenuhi interface.
type noopNotifier struct{}

func (noopNotifier) Notify(ctx context.Context, rule *domain.AlertRule, issue *domain.Issue) error {
	return nil
}

type testDeps struct {
	pool        *pgxpool.Pool
	redisClient *redis.Client
	userID      string
}

// newConsumerFunc -- factory, dipakai karena tiap test butuh kombinasi
// maxDeliveryCount/reclaimMinIdle/reclaimInterval yang berbeda-beda
// (test cepat butuh angka kecil), tapi semuanya berbagi dependency
// Postgres+Redis yang SAMA (satu container per test function, bukan per
// consumer).
func setupDeps(t *testing.T) (testDeps, func(maxDeliveryCount int64, reclaimMinIdle, reclaimInterval time.Duration) *worker.IngestConsumer) {
	t.Helper()

	pool := testutil.NewPostgresPool(t)
	redisClient := testutil.NewRedisClient(t)

	userRepo := postgres.NewUserRepository(pool)
	issueRepo := postgres.NewIssueRepository(pool)
	eventRepo := postgres.NewEventRepository(pool)
	alertRuleRepo := postgres.NewAlertRuleRepository(pool)
	alertLogRepo := postgres.NewAlertLogRepository(pool)

	user := &domain.User{Email: fmt.Sprintf("test-%d@example.com", time.Now().UnixNano()), PasswordHash: "hash"}
	require.NoError(t, userRepo.Create(context.Background(), user))

	groupIssueUsecase := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	evaluateAlertUsecase := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, noopNotifier{})
	broadcaster := redisrepo.NewBroadcaster(redisClient)

	newConsumer := func(maxDeliveryCount int64, reclaimMinIdle, reclaimInterval time.Duration) *worker.IngestConsumer {
		return worker.NewIngestConsumer(
			redisClient, zerolog.Nop(), groupIssueUsecase, broadcaster, evaluateAlertUsecase,
			maxDeliveryCount, reclaimMinIdle, reclaimInterval,
		)
	}

	return testDeps{pool: pool, redisClient: redisClient, userID: user.ID}, newConsumer
}

// insertProjectWithID -- raw SQL, BUKAN lewat ProjectRepository.Create()
// (yang selalu generate ID sendiri lewat gen_random_uuid() di DB). Test
// "succeeds once project exists" butuh ID yang SUDAH DIKETAHUI SEBELUM
// row-nya benar-benar ada, supaya bisa kirim event yang mengacu ke ID itu
// LEBIH DULU, baru insert project-nya belakangan (simulasi "masalah
// pulih").
func insertProjectWithID(t *testing.T, pool *pgxpool.Pool, id, userID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO projects (id, user_id, name, slug, api_key_hash)
		VALUES ($1, $2, $3, $4, $5)
	`, id, userID, "Delayed Project", fmt.Sprintf("delayed-%d", time.Now().UnixNano()), fmt.Sprintf("hash-%d", time.Now().UnixNano()))
	require.NoError(t, err)
}

func pushRawEvent(t *testing.T, client *redis.Client, dataField string) {
	t.Helper()
	_, err := client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "queue:ingest",
		Values: map[string]interface{}{"data": dataField},
	}).Result()
	require.NoError(t, err)
}

// waitUntil -- polling helper, karena Run()/RunReclaim() jalan di
// goroutine terpisah secara async. Timeout 5 detik jauh di atas
// reclaimMinIdle/reclaimInterval yang dipakai test-test di bawah (semua
// dalam hitungan puluh-ratusan milidetik), jadi bukan flaky-by-design.
func waitUntil(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("condition not met within %s", timeout)
}

func dlqLength(t *testing.T, client *redis.Client) int64 {
	t.Helper()
	length, err := client.XLen(context.Background(), "queue:ingest:dlq").Result()
	require.NoError(t, err)
	return length
}

func pendingCount(t *testing.T, client *redis.Client) int64 {
	t.Helper()
	pending, err := client.XPending(context.Background(), "queue:ingest", "ingest-workers").Result()
	require.NoError(t, err)
	return pending.Count
}

func TestIngestConsumer_MalformedMessage_MovesToDLQImmediately(t *testing.T) {
	deps, newConsumer := setupDeps(t)
	consumer := newConsumer(3, 100*time.Millisecond, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = consumer.Run(ctx) }()

	pushRawEvent(t, deps.redisClient, "{not valid json")

	waitUntil(t, 5*time.Second, func() bool { return dlqLength(t, deps.redisClient) == 1 })
	assert.Equal(t, int64(0), pendingCount(t, deps.redisClient),
		"pesan malformed harus langsung ke-ack ke DLQ, tidak boleh nyangkut di pending")
}

func TestIngestConsumer_TransientFailure_MovesToDLQAfterMaxRetries(t *testing.T) {
	deps, newConsumer := setupDeps(t)
	// maxDeliveryCount=1: kasih 1x kesempatan retry dulu (jadi total 2x
	// percobaan) sebelum ke DLQ -- bukan langsung DLQ di attempt pertama.
	consumer := newConsumer(1, 50*time.Millisecond, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = consumer.Run(ctx) }()
	go func() { _ = consumer.RunReclaim(ctx) }()

	// ProjectID UUID acak yang TIDAK PERNAH ada di tabel projects --
	// groupIssue.Execute SELALU gagal (FK violation), simulasi kegagalan
	// transient yang tidak pernah pulih sendiri.
	event := domain.Event{
		ProjectID:  "00000000-0000-0000-0000-000000000000",
		Level:      "error",
		Message:    "simulated persistent failure",
		OccurredAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	pushRawEvent(t, deps.redisClient, string(raw))

	waitUntil(t, 5*time.Second, func() bool { return dlqLength(t, deps.redisClient) == 1 })
	assert.Equal(t, int64(0), pendingCount(t, deps.redisClient),
		"setelah masuk DLQ, pesan harus ke-ack dari stream utama")
}

func TestIngestConsumer_TransientFailure_SucceedsOnceProjectExists(t *testing.T) {
	deps, newConsumer := setupDeps(t)
	consumer := newConsumer(5, 50*time.Millisecond, 100*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = consumer.Run(ctx) }()
	go func() { _ = consumer.RunReclaim(ctx) }()

	projectID := "11111111-1111-1111-1111-111111111111"
	event := domain.Event{
		ProjectID:  projectID,
		Level:      "error",
		Message:    "event arrives before project row exists",
		OccurredAt: time.Now().UTC(),
	}
	raw, err := json.Marshal(event)
	require.NoError(t, err)
	pushRawEvent(t, deps.redisClient, string(raw))

	// Kasih waktu attempt pertama gagal & pesan jadi pending dulu, SEBELUM
	// project row-nya benar-benar dibuat.
	time.Sleep(200 * time.Millisecond)
	require.Equal(t, int64(1), pendingCount(t, deps.redisClient),
		"sebelum project dibuat, event harus gagal & tetap pending (bukan hilang, bukan juga sukses)")

	// "Masalahnya pulih" -- project row sekarang benar-benar ada, dengan
	// ID yang SAMA PERSIS dengan yang sudah dipakai event di atas.
	insertProjectWithID(t, deps.pool, projectID, deps.userID)

	waitUntil(t, 5*time.Second, func() bool { return pendingCount(t, deps.redisClient) == 0 })
	assert.Equal(t, int64(0), dlqLength(t, deps.redisClient),
		"event yang akhirnya berhasil di-retry TIDAK BOLEH nyasar ke DLQ")
}
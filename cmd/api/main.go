package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/MohdFarhanS/sentinelix-backend/config"
	deliveryhttp "github.com/MohdFarhanS/sentinelix-backend/internal/delivery/http"
	"github.com/MohdFarhanS/sentinelix-backend/internal/delivery/ws"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/MohdFarhanS/sentinelix-backend/pkg/jwt"
	"github.com/rs/zerolog"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	if err := dbPool.Ping(ctx); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	redisOpt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	redisClient := redis.NewClient(redisOpt)
	defer redisClient.Close()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Fatalf("failed to ping redis: %v", err)
	}

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// Wiring auth (Sprint 1)
	userRepo := postgres.NewUserRepository(dbPool)
	jwtManager := jwt.NewManager(cfg.JWTSecret, time.Hour)
	authUsecase := usecase.NewAuthUsecase(userRepo, jwtManager)
	authHandler := deliveryhttp.NewAuthHandler(authUsecase, cfg.Env == "production")

	// Wiring ingest (Sprint 2) — projectRepo di-reuse di bawah buat project & issue usecase
	projectRepo := postgres.NewProjectRepository(dbPool)
	rateLimiter := redisrepo.NewRateLimiter(redisClient)
	eventQueue := redisrepo.NewEventQueue(redisClient)
	ingestUsecase := usecase.NewIngestEventUsecase(projectRepo, rateLimiter, eventQueue)
	ingestHandler := deliveryhttp.NewIngestHandler(ingestUsecase)

	// Wiring WebSocket hub (Sprint 5) — broadcaster di-reuse lagi di
	// bawah (Sprint 7) buat MonitorSyncPublisher, satu Redis channel
	// (ws:broadcast) dipakai dua concern berbeda.
	hub := ws.NewHub(logger)
	broadcaster := redisrepo.NewBroadcaster(redisClient)
	go ws.RunSubscriber(ctx, hub, broadcaster)

	// Wiring project & issue (prasyarat Sprint 4)
	issueRepo := postgres.NewIssueRepository(dbPool)
	eventRepo := postgres.NewEventRepository(dbPool)
	projectUsecase := usecase.NewProjectUsecase(projectRepo)
	issueUsecase := usecase.NewIssueUsecase(issueRepo, projectRepo, eventRepo)
	wsHandler := deliveryhttp.NewWSHandler(hub, projectUsecase, logger)
	projectHandler := deliveryhttp.NewProjectHandler(projectUsecase)
	issueHandler := deliveryhttp.NewIssueHandler(issueUsecase)

	// Wiring alert rule (Sprint 6) — cuma CRUD di sini (delivery HTTP).
	// EvaluateAlertUsecase (yang butuh Notifier) TIDAK di-wire di sini
	// sama sekali — itu murni concern-nya cmd/worker/main.go, API server
	// tidak pernah mengevaluasi atau mengirim alert secara langsung.
	alertRuleRepo := postgres.NewAlertRuleRepository(dbPool)
	alertRuleUsecase := usecase.NewAlertRuleUsecase(alertRuleRepo, projectRepo)
	alertRuleHandler := deliveryhttp.NewAlertRuleHandler(alertRuleUsecase)

	// Wiring monitor (Sprint 7) — sama prinsipnya seperti alert rule:
	// cuma CRUD di sini. MonitorCheckerUsecase (yang butuh MonitorNotifier
	// & benar-benar ping URL) TIDAK di-wire di sini — itu concern
	// cmd/worker/main.go. broadcaster di-pass sebagai MonitorSyncPublisher
	// (Publish signature-nya sudah cocok, tidak perlu adapter).
	monitorRepo := postgres.NewMonitorRepository(dbPool)
	monitorCheckRepo := postgres.NewMonitorCheckRepository(dbPool)
	monitorUsecase := usecase.NewMonitorUsecase(monitorRepo, monitorCheckRepo, projectRepo, broadcaster)
	monitorHandler := deliveryhttp.NewMonitorHandler(monitorUsecase, logger)

	// CHANGED: router sekarang butuh monitorHandler juga
	router := deliveryhttp.NewRouter(
		authHandler,
		ingestHandler,
		projectHandler,
		issueHandler,
		alertRuleHandler,
		monitorHandler,
		wsHandler,
		jwtManager,
		cfg.FrontendURL,
	)

	log.Printf("server running on port %s (env: %s)", cfg.Port, cfg.Env)
	if err := http.ListenAndServe(":"+cfg.Port, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
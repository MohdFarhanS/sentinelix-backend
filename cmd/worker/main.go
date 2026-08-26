package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/config"
	"github.com/MohdFarhanS/sentinelix-backend/internal/notifier"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	redisrepo "github.com/MohdFarhanS/sentinelix-backend/internal/repository/redis"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
	"github.com/MohdFarhanS/sentinelix-backend/internal/worker"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}
	if cfg.ResendAPIKey == "" {
		log.Fatalf("RESEND_API_KEY is required for worker (email channel notifications)")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// --- Postgres ---
	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer dbPool.Close()

	// --- Redis ---
	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	opt.ReadTimeout = 7 * time.Second
	redisClient := redis.NewClient(opt)

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	// --- Wiring: repository ---
	issueRepo := postgres.NewIssueRepository(dbPool)
	eventRepo := postgres.NewEventRepository(dbPool)
	alertRuleRepo := postgres.NewAlertRuleRepository(dbPool)
	alertLogRepo := postgres.NewAlertLogRepository(dbPool)
	monitorRepo := postgres.NewMonitorRepository(dbPool)
	monitorCheckRepo := postgres.NewMonitorCheckRepository(dbPool)

	// broadcaster dipindah ke atas (sebelum monitorCheckerUsecase) —
	// sekarang dibutuhkan sebagai dependency-nya, bukan cuma buat
	// consumer & monitor sync subscriber seperti sebelumnya.
	broadcaster := redisrepo.NewBroadcaster(redisClient)

	// --- Wiring: usecase ---
	groupIssueUsecase := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	// multiNotifier mengimplementasikan DUA interface sekaligus:
	// usecase.Notifier (buat EvaluateAlertUsecase, alert issue) DAN
	// usecase.MonitorNotifier (buat MonitorCheckerUsecase, monitor down)
	// — satu instance HTTP client di-reuse buat kedua concern.
	multiNotifier := notifier.NewMultiNotifier(cfg.ResendAPIKey, cfg.EmailFromAddress)
	evaluateAlertUsecase := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, multiNotifier)
	// broadcaster di-pass juga ke monitorCheckerUsecase — dipakai buat
	// broadcast monitor.status_changed ke dashboard client (BUKAN cuma
	// buat monitor sync internal, lihat komentar di check_monitor.go).
	monitorCheckerUsecase := usecase.NewMonitorCheckerUsecase(monitorRepo, monitorCheckRepo, multiNotifier, broadcaster)

	// --- Wiring: worker ---
	consumer := worker.NewIngestConsumer(redisClient, logger, groupIssueUsecase, broadcaster, evaluateAlertUsecase)
	alertNotifierWorker := worker.NewAlertNotifierWorker(evaluateAlertUsecase, logger, time.Minute)
	monitorSupervisor := worker.NewMonitorSupervisor(monitorRepo, monitorCheckerUsecase, logger)

	// Jalankan 4 loop paralel. errCh nampung error PERTAMA yang muncul
	// dari salah satu goroutine — begitu satu loop mati karena error
	// (bukan context.Canceled), proses utama ikut exit.
	errCh := make(chan error, 4)
	go func() { errCh <- consumer.Run(ctx) }()
	go func() { errCh <- alertNotifierWorker.Run(ctx) }()
	go func() { errCh <- monitorSupervisor.Run(ctx) }()
	go func() { errCh <- worker.RunMonitorSync(ctx, monitorSupervisor, broadcaster, logger) }()

	if err := <-errCh; err != nil && err != context.Canceled {
		logger.Fatal().Err(err).Msg("worker stopped with error")
	}
}

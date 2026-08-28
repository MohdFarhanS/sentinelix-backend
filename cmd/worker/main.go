package main

import (
	"context"
	"log"
	"net/http"
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

	dbPool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}
	defer dbPool.Close()

	opt, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		log.Fatalf("invalid REDIS_URL: %v", err)
	}
	opt.ReadTimeout = 7 * time.Second
	redisClient := redis.NewClient(opt)

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	issueRepo := postgres.NewIssueRepository(dbPool)
	eventRepo := postgres.NewEventRepository(dbPool)
	alertRuleRepo := postgres.NewAlertRuleRepository(dbPool)
	alertLogRepo := postgres.NewAlertLogRepository(dbPool)
	monitorRepo := postgres.NewMonitorRepository(dbPool)
	monitorCheckRepo := postgres.NewMonitorCheckRepository(dbPool)

	broadcaster := redisrepo.NewBroadcaster(redisClient)

	groupIssueUsecase := usecase.NewGroupIssueUsecase(issueRepo, eventRepo)
	multiNotifier := notifier.NewMultiNotifier(cfg.ResendAPIKey, cfg.EmailFromAddress)
	evaluateAlertUsecase := usecase.NewEvaluateAlertUsecase(alertRuleRepo, alertLogRepo, issueRepo, eventRepo, multiNotifier)
	monitorCheckerUsecase := usecase.NewMonitorCheckerUsecase(monitorRepo, monitorCheckRepo, multiNotifier, broadcaster)

	consumer := worker.NewIngestConsumer(redisClient, logger, groupIssueUsecase, broadcaster, evaluateAlertUsecase)
	// Interval dinaikkan dari 1 menit -> 10 menit (Sprint 10, audit compute
	// Neon). Ini CUMA mempengaruhi alert condition_type="threshold" (deteksi
	// lonjakan sustained dalam window waktu) — alert condition_type=
	// "new_issue" TETAP instant, karena itu event-driven langsung dari
	// ingest_consumer di atas, tidak lewat ticker ini sama sekali. Target
	// PRD "<60 detik dari error ke notifikasi" (01-PRD.md §7) tidak
	// terdampak, karena metrik itu diukur dari jalur new_issue.
	//
	// Alasan diperbesar: ticker 1 menit bikin Neon TIDAK PERNAH sempat
	// idle 5 menit buat auto-suspend (lihat README backend, "A deliberate
	// trade-off") -- 10 menit ngasih jeda jelas di atas threshold itu.
	alertNotifierWorker := worker.NewAlertNotifierWorker(evaluateAlertUsecase, logger, 10*time.Minute)
	monitorSupervisor := worker.NewMonitorSupervisor(monitorRepo, monitorCheckerUsecase, logger)

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}
	healthServer := &http.Server{Addr: ":" + port, Handler: healthMux}

	go func() {
		<-ctx.Done()
		_ = healthServer.Shutdown(context.Background())
	}()

	log.Printf("worker healthz endpoint running on port %s (env: %s)", port, cfg.Env)

	errCh := make(chan error, 5)
	go func() { errCh <- consumer.Run(ctx) }()
	go func() { errCh <- alertNotifierWorker.Run(ctx) }()
	go func() { errCh <- monitorSupervisor.Run(ctx) }()
	go func() { errCh <- worker.RunMonitorSync(ctx, monitorSupervisor, broadcaster, logger) }()
	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if err := <-errCh; err != nil && err != context.Canceled {
		logger.Fatal().Err(err).Msg("worker stopped with error")
	}
}
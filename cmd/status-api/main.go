package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"

	"github.com/MohdFarhanS/sentinelix-backend/config"
	"github.com/MohdFarhanS/sentinelix-backend/internal/delivery"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
	"github.com/MohdFarhanS/sentinelix-backend/internal/usecase"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	ctx := context.Background()

	// pgxpool config KHUSUS status-api — MinConns=0 supaya pool TIDAK
	// mempertahankan koneksi idle, sehingga Neon compute bebas suspend
	// saat tidak ada trafik status page beneran (lihat 05-ARCHITECTURE.md
	// §6c). BEDA SENGAJA dari cmd/api/cmd/worker yang traffic-nya lebih
	// kontinu/predictable.
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("invalid DATABASE_URL: %v", err)
	}
	poolConfig.MinConns = 0
	poolConfig.MaxConns = 2
	poolConfig.MaxConnIdleTime = 30 * time.Second
	poolConfig.MaxConnLifetime = 5 * time.Minute

	dbPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	// SENGAJA TIDAK Ping() database saat startup (beda dari cmd/api) —
	// Ping saat boot akan langsung membangunkan Neon compute walau belum
	// ada trafik status page beneran, bertentangan dengan tujuan
	// MinConns=0 di atas. Koneksi pertama baru kebuka natural saat ada
	// request GET /api/v1/status/:slug beneran.

	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	statusRepo := postgres.NewStatusRepository(dbPool)
	getStatusPageUsecase := usecase.NewGetStatusPageUsecase(statusRepo)
	statusHandler := delivery.NewStatusHandler(getStatusPageUsecase, logger)

	router := delivery.NewStatusRouter(statusHandler)

	// STATUS_API_PORT (BUKAN PORT) — sengaja nama beda dari PORT yang
	// dipakai cmd/api. Kalau reuse "PORT", os.Getenv("PORT") akan kebaca
	// dari .env yang sama (godotenv.Load() dipanggil config.Load(), jadi
	// SEMUA proses share .env file yang sama) — otomatis bentrok kalau
	// dijalankan bareng cmd/api di local, walau niatnya beda default.
	port := os.Getenv("STATUS_API_PORT")
	if port == "" {
		port = "8081"
	}

	log.Printf("status-api running on port %s (env: %s)", port, cfg.Env)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("status-api server failed: %v", err)
	}
}

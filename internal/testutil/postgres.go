package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// NewPostgresPool spin up container Postgres BARU per pemanggilan (bukan
// di-share antar test) — trade-off disengaja: ~15-20 fungsi test
// repository di project ini nambah total ~45-60 detik overhead start-up
// container dibanding container yang di-share (TestMain + TRUNCATE), tapi
// hindarin kompleksitas ordering TRUNCATE demi isolasi test yang
// sepenuhnya independen. Kalau nanti overhead ini kerasa mengganggu di CI,
// pindah ke shared container TIDAK butuh ubah assertion di tiap test —
// SELAMA tiap test cuma query row yang dia buat sendiri (pakai ID hasil
// Create, BUKAN `SELECT * FROM table` tanpa filter).
func NewPostgresPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, 
		"postgres:16-alpine",
		tcpostgres.WithDatabase("sentinelix_test"),
		tcpostgres.WithUsername("sentinelix"),
		tcpostgres.WithPassword("sentinelix"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(context.Background()); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}

	runMigrations(t, connStr)

	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatalf("failed to create pgxpool: %v", err)
	}
	t.Cleanup(pool.Close)
	
	return pool
}

// runMigrations jalankan SEMUA migration dari folder migrations/ ke
// container yang baru dibuat — test dapat skema yang sinkron sama
// migration terbaru, bukan skema hardcoded/asumsi yang bisa basi.
func runMigrations(t *testing.T, connStr string) {
	t.Helper()

	// Path relatif dari package test manapun yang manggil helper ini
	// (internal/repository/postgres/ ATAU internal/repository/redis/)
	// ke folder migrations/ di root repo — SAMA-SAMA 3 level naik.
	migrationsPath := "file://../../../migrations"

	m, err := migrate.New(migrationsPath, connStr)
	if err != nil {
		t.Fatalf("failed to init migrate: %v", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}
}
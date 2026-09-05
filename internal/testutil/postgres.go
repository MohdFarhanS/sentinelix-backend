package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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

// migrationsDir — path OS-native (BUKAN string URL), dihitung dari lokasi
// FILE INI SENDIRI (runtime.Caller), bukan lokasi package yang MEMANGGIL
// helper ini. Ini yang bikin resolusi path BENAR untuk caller di
// kedalaman manapun (internal/repository/postgres/, internal/worker/,
// dst) — beda dari versi lama yang hardcode "../../../" dan asumsi semua
// caller persis 3 level dari root.
func migrationsDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "..", "..", "migrations")
}

// runMigrations — SENGAJA pakai source/iofs + os.DirFS(), BUKAN
// source/file dengan string URL ("file://...."). Alasan: merepresentasikan
// path lokal sebagai string URI ("file:///D:/...") terbukti rapuh di
// Windows lewat 2 percobaan berturut-turut (drive letter "D:" ketuker
// parser jadi host:port di percobaan pertama, lalu "The filename,
// directory name, or volume label syntax is incorrect" di percobaan
// kedua walau formatnya sudah sesuai dokumentasi resmi golang-migrate).
//
// os.DirFS() + iofs.New() SAMA SEKALI TIDAK PERNAH melewati path lokal
// sebagai string URL — Go runtime yang menerjemahkan path OS-native ke
// fs.FS secara internal (fs.FS SELALU pakai forward-slash secara
// spesifikasi, apapun OS-nya), jadi tidak ada celah salah-parsing drive
// letter sama sekali. Ini juga cara yang didokumentasikan resmi
// golang-migrate sejak v4.15.0 (project ini pakai v4.19.1).
func runMigrations(t *testing.T, connStr string) {
	t.Helper()

	fsys := os.DirFS(migrationsDir())
	sourceDriver, err := iofs.New(fsys, ".")
	if err != nil {
		t.Fatalf("failed to init iofs source: %v", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", sourceDriver, connStr)
	if err != nil {
		t.Fatalf("failed to init migrate: %v", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatalf("failed to run migrations: %v", err)
	}
}
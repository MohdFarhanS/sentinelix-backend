package domain

import (
	"context"
	"errors"
	"time"
)

// Issue merepresentasikan satu grup error yang sudah di-fingerprint.
// Sinkron dengan tabel `issues` di 03-DATABASE-DESIGN.md.
type Issue struct {
	ID          string
	ProjectID   string
	Fingerprint string
	Title       string
	Level       string
	Status      string
	FirstSeen   time.Time
	LastSeen    time.Time
	Count       int
}

type IssueFilter struct {
	ProjectID string
	Status    string
	Page      int
	Limit     int
}

type IssueListResult struct {
	Issues []*Issue
	Total  int
}

const (
	IssueStatusUnresolved = "unresolved"
	IssueStatusResolved   = "resolved"
	IssueStatusIgnored    = "ignored"
)

var ErrIssueNotFound = errors.New("issue not found")

// IssueRepository didefinisikan di domain (dependency mengarah ke dalam),
// diimplementasikan konkret di internal/repository/postgres.
type IssueRepository interface {
	// Upsert insert issue baru ATAU update issue yang sudah ada (match by
	// project_id + fingerprint) dalam SATU query atomik — supaya aman dari
	// race condition kalau nanti worker di-scale >1 instance (lihat NFR-6
	// di 02-REQUIREMENTS.md: "horizontal scaling worker tanpa duplikasi").
	//
	// title & level cuma dipakai kalau ini insert baru; issue lama tidak
	// akan berubah title/level-nya walau event baru datang dengan title
	// sedikit beda.
	//
	// Return wasCreated=true kalau ini issue BARU — dipakai nanti di
	// usecase evaluate_alert buat trigger rule "new_issue" (di luar scope
	// Sprint 3, tapi kita siapkan return value-nya dari sekarang).
	Upsert(ctx context.Context, projectID, fingerprint, title, level string, occurredAt time.Time) (issue *Issue, wasCreated bool, err error)

	// List dipakai buat GET /projects/:projectId/issues. Total dihitung
	// pakai window function COUNT(*) OVER() supaya cuma 1 round-trip
	// (bukan query count terpisah), sesuai NFR-2 (query < 300ms p95).
	List(ctx context.Context, filter IssueFilter) (*IssueListResult, error)

	// GetByID dipakai buat GET /issues/:id. Return domain.ErrIssueNotFound
	// kalau row tidak ditemukan, biar handler bisa balikin 404 secara akurat
	GetByID(ctx context.Context, id string) (*Issue, error)
}

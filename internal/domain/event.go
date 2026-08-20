package domain

import (
	"context"
	"errors"
	"time"
)

type Event struct {
	ID			string
	IssueID		string
	ProjectID	string
	Level		string
	Message		string
	StackTrace	string
	Context		map[string]any
	OccurredAt	time.Time
}

var (
	ErrEventMessageRequired = errors.New("message is required")
	ErrEventLevelInvalid = errors. New("level is invalid")
)

var validLevels = map[string]bool{
	"error": 	true,
	"warning":	true,
	"info": 	true,
}

func (e *Event) Validate() error {
	if e.Message == "" {
		return ErrEventMessageRequired
	}
	if e.Level == "" {
		e.Level = "error"
	}
	if !validLevels[e.Level] {
		return ErrEventLevelInvalid
	}
	return nil
}

// EventRepository didefinisikan di domain, diimplementasikan di
// internal/repository/postgres.
type EventRepository interface {
	// Insert menyimpan satu event mentah. event.IssueID WAJIB sudah diisi
	// (hasil IssueRepository.Upsert) sebelum method ini dipanggil.
	Insert(ctx context.Context, event *Event) error

	// ListByIssueID mengambil event terbaru untuk satu issue, diurutkan
	// occurred_at DESC (event paling baru duluan), dibatasi 'limit'
	// (max 50 sesuai FR-11 di 02-REQUIREMENTS.md, no cursor pagination v1).
	ListByIssueID(ctx context.Context, issueID string, limit int) ([]*Event, error)

	// CountGroupedByIssueSince menghitung jumlah event per issue dalam
	// satu project, sejak waktu 'since', dengan SATU query GROUP BY —
	// dipakai worker threshold ticker (Sprint 6) buat evaluasi
	// windowed count tanpa N+1 query per issue. Issue dengan 0 event
	// dalam window TIDAK muncul di map hasil (bukan key dengan value 0).
	CountGroupedByIssueSince(ctx context.Context, projectID string, since time.Time) (map[string]int, error)
}
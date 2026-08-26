package domain

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

type Event struct {
	ID         string
	IssueID    string
	ProjectID  string
	Level      string
	Message    string
	StackTrace string
	Context    map[string]any
	OccurredAt time.Time
}

var (
	ErrEventMessageRequired   = errors.New("message is required")
	ErrEventLevelInvalid      = errors.New("level is invalid")
	ErrEventMessageTooLong    = errors.New("message exceeds maximum length of 2000 characters")
	ErrEventStackTraceTooLong = errors.New("stack_trace exceeds maximum length of 20000 characters")
	ErrEventContextTooLarge   = errors.New("context exceeds maximum serialized size of 10KB")
)

var validLevels = map[string]bool{
	"error":   true,
	"warning": true,
	"info":    true,
}

// Batas ukuran payload (Sprint 9, 06-ROADMAP.md §6 "Validasi & sanitasi
// payload event"). Dicek pakai len(string) — byte length, bukan rune
// count — ini SENGAJA: tujuannya proteksi storage/memory (events table
// partitioned per bulan, 03-DATABASE-DESIGN.md), byte length representasi
// langsung dari ukuran yang beneran dipakai di DB/memory, bukan cuma
// "jumlah karakter" yang kurang relevan buat kasus ini.
const (
	maxMessageLength    = 2000
	maxStackTraceLength = 20000
	maxContextBytes     = 10 * 1024 // 10 KB
)

func (e *Event) Validate() error {
	if e.Message == "" {
		return ErrEventMessageRequired
	}
	if len(e.Message) > maxMessageLength {
		return ErrEventMessageTooLong
	}
	if len(e.StackTrace) > maxStackTraceLength {
		return ErrEventStackTraceTooLong
	}
	if e.Context != nil {
		// Cek ukuran hasil serialize, bukan hitung field manual — lebih
		// simpel & langsung representatif terhadap ukuran yang benar-benar
		// disimpan di kolom JSONB, termasuk cegah nested object super
		// dalam ("JSON bomb") yang secara teks pendek tapi struktur berat.
		serialized, err := json.Marshal(e.Context)
		if err != nil {
			return err
		}
		if len(serialized) > maxContextBytes {
			return ErrEventContextTooLarge
		}
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

package domain_test

import (
	"strings"
	"testing"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

// validEvent — fixture standar dipakai semua test yang BUKAN spesifik
// nguji satu field tertentu, supaya test itu fokus ke satu hal yang diuji.
func validEvent() *domain.Event {
	return &domain.Event{
		Level:      "error",
		Message:    "TypeError: Cannot read property 'id' of undefined",
		StackTrace: "at checkout.go:88",
		Context:    map[string]any{"user_id": "123", "route": "/checkout"},
	}
}

func TestEventValidate_Success(t *testing.T) {
	e := validEvent()
	if err := e.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestEventValidate_MessageRequired(t *testing.T) {
	e := validEvent()
	e.Message = ""

	err := e.Validate()
	if err != domain.ErrEventMessageRequired {
		t.Errorf("expected ErrEventMessageRequired, got %v", err)
	}
}

func TestEventValidate_LevelDefaultsToError(t *testing.T) {
	e := validEvent()
	e.Level = ""

	if err := e.Validate(); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if e.Level != "error" {
		t.Errorf("expected level to default to 'error', got %q", e.Level)
	}
}

func TestEventValidate_InvalidLevel(t *testing.T) {
	e := validEvent()
	e.Level = "critical" // bukan salah satu dari error|warning|info

	err := e.Validate()
	if err != domain.ErrEventLevelInvalid {
		t.Errorf("expected ErrEventLevelInvalid, got %v", err)
	}
}

func TestEventValidate_MessageTooLong(t *testing.T) {
	e := validEvent()
	e.Message = strings.Repeat("a", 2001) // 1 karakter di atas batas

	err := e.Validate()
	if err != domain.ErrEventMessageTooLong {
		t.Errorf("expected ErrEventMessageTooLong, got %v", err)
	}
}

func TestEventValidate_MessageAtMaxLength(t *testing.T) {
	// Boundary test — TEPAT di batas (2000) harus tetap LOLOS, bukan
	// ditolak. Cegah regresi off-by-one (misal kalau nanti berubah dari
	// > jadi >=).
	e := validEvent()
	e.Message = strings.Repeat("a", 2000)

	if err := e.Validate(); err != nil {
		t.Errorf("expected message at exactly max length to pass, got %v", err)
	}
}

func TestEventValidate_StackTraceTooLong(t *testing.T) {
	e := validEvent()
	e.StackTrace = strings.Repeat("a", 20001)

	err := e.Validate()
	if err != domain.ErrEventStackTraceTooLong {
		t.Errorf("expected ErrEventStackTraceTooLong, got %v", err)
	}
}

func TestEventValidate_StackTraceAtMaxLength(t *testing.T) {
	e := validEvent()
	e.StackTrace = strings.Repeat("a", 20000)

	if err := e.Validate(); err != nil {
		t.Errorf("expected stack trace at exactly max length to pass, got %v", err)
	}
}

func TestEventValidate_ContextTooLarge(t *testing.T) {
	e := validEvent()
	// String 11000 karakter di dalam satu field JSON sudah pasti lewat
	// batas 10KB (10240 byte) setelah di-marshal (nambah quote, key, dst).
	e.Context = map[string]any{"payload": strings.Repeat("x", 11000)}

	err := e.Validate()
	if err != domain.ErrEventContextTooLarge {
		t.Errorf("expected ErrEventContextTooLarge, got %v", err)
	}
}

func TestEventValidate_ContextNilAllowed(t *testing.T) {
	// Context itu opsional (lihat 04-API-DESIGN.md §4 — field ini ada di
	// contoh request tapi tidak didokumentasikan sebagai wajib). nil
	// harus lolos validasi, bukan dianggap "context kosong yang invalid".
	e := validEvent()
	e.Context = nil

	if err := e.Validate(); err != nil {
		t.Errorf("expected nil context to pass, got %v", err)
	}
}

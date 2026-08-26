package domain

import (
	"context"
	"errors"
	"time"
)

type Project struct {
	ID         string
	UserID     string
	Name       string
	Slug       string
	APIKeyHash string
	CreatedAt  time.Time
}

var ErrProjectNotFound = errors.New("project not found")

type ProjectRepository interface {
	GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*Project, error)

	// Create insert project baru. p.ID dan p.CreatedAt di-generate DB
	// (gen_random_uuid(), now()), di-scan balik ke pointer p setelah insert.
	Create(ctx context.Context, p *Project) error

	// GetByID dipakai buat cek ownership sebelum list issues — project ini
	// beneran milik user yang login, bukan cuma exist.
	GetByID(ctx context.Context, id string) (*Project, error)

	// ListByUserID dipakai buat GET /projects.
	ListByUserID(ctx context.Context, userID string) ([]*Project, error)

	// Delete hapus project. Cascade ke issues/events/alert_rules/monitors
	// SEPENUHNYA ditangani DB (ON DELETE CASCADE, lihat
	// 03-DATABASE-DESIGN.md) — implementasi ini TIDAK perlu manual hapus
	// child records satu-satu, satu DELETE statement cukup.
	Delete(ctx context.Context, id string) error
}
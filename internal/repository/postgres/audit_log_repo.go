package postgres

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type AuditLogRepository struct {
	db *pgxpool.Pool
}

func NewAuditLogRepository(db *pgxpool.Pool) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// Create — satu-satunya method di interface ini (lihat domain.AuditLogRepository),
// tidak ada Read/List — audit log di scope Sprint 9 ini cuma perlu ditulis,
// belum ada UI/endpoint buat query histori-nya (kalau nanti dibutuhkan,
// itu penambahan method terpisah, bukan alasan buat generalize interface
// ini sekarang).
func (r *AuditLogRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	// Metadata bisa nil (bukan semua action perlu detail tambahan) —
	// json.Marshal(nil map) hasilnya "null", valid buat kolom JSONB.
	metadataJSON, err := json.Marshal(log.Metadata)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO audit_logs (actor_user_id, action, resource_type, resource_id, metadata)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	return r.db.QueryRow(ctx, query,
		log.ActorUserID, log.Action, log.ResourceType, log.ResourceID, metadataJSON,
	).Scan(&log.ID, &log.CreatedAt)
}

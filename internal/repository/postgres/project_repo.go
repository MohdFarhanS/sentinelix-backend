package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
)

type ProjectRepository struct {
	db *pgxpool.Pool
}

func NewProjectRepository(db *pgxpool.Pool) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) GetByAPIKeyHash(ctx context.Context, apiKeyHash string) (*domain.Project, error) {
	query := `
		SELECT id, user_id, name, slug, api_key_hash, created_at
		FROM projects
		WHERE api_key_hash = $1
	`

	var p domain.Project
	err := r.db.QueryRow(ctx, query, apiKeyHash).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Slug, &p.APIKeyHash, &p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepository) Create(ctx context.Context, p *domain.Project) error {
	query := `
		INSERT INTO projects (user_id, name, slug, api_key_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at
	`
	return r.db.QueryRow(ctx, query, p.UserID, p.Name, p.Slug, p.APIKeyHash).
		Scan(&p.ID, &p.CreatedAt)
}

func (r *ProjectRepository) GetByID(ctx context.Context, id string) (*domain.Project, error) {
	query := `
		SELECT id, user_id, name, slug, api_key_hash, created_at
		FROM projects
		WHERE id = $1
	`
	var p domain.Project
	err := r.db.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.UserID, &p.Name, &p.Slug, &p.APIKeyHash, &p.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrProjectNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *ProjectRepository) ListByUserID(ctx context.Context, userID string) ([]*domain.Project, error) {
	query := `
		SELECT id, user_id, name, slug, api_key_hash, created_at
		FROM projects
		WHERE user_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*domain.Project
	for rows.Next() {
		var p domain.Project
		if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Slug, &p.APIKeyHash, &p.CreatedAt); err != nil {
			return nil, err
		}
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

func (r *ProjectRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM projects WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	return err
}
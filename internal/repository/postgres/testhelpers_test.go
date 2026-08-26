package postgres_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/MohdFarhanS/sentinelix-backend/internal/domain"
	"github.com/MohdFarhanS/sentinelix-backend/internal/repository/postgres"
)

// createTestUser — helper dipakai bareng SEMUA file test di package ini
// (refresh_token_repo_test.go, audit_log_repo_test.go, project_repo_test.go)
// — semuanya butuh baris users.id valid buat FK.
func createTestUser(t *testing.T, pool *pgxpool.Pool) *domain.User {
	t.Helper()
	repo := postgres.NewUserRepository(pool)

	user := &domain.User{
		Email:        fmt.Sprintf("test-%d@sentinelix.com", time.Now().UnixNano()),
		PasswordHash: "irrelevant-for-repository-test",
	}
	err := repo.Create(context.Background(), user)
	require.NoError(t, err)

	return user
}
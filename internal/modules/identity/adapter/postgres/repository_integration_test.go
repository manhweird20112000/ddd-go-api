//go:build integration

package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/yourorg/go-template/internal/modules/identity/adapter/postgres"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	"github.com/yourorg/go-template/internal/modules/identity/migrations"
)

func TestRepositoryWithPostgres(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pgcontainer.Run(ctx, "postgres:16",
		pgcontainer.BasicWaitStrategies(),
		pgcontainer.WithDatabase("identity_test"),
		pgcontainer.WithUsername("identity_test"),
		pgcontainer.WithPassword(uuid.NewString()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, migrations.Run(ctx, databaseURL, "up"))
	pool, err := pgxpool.New(ctx, databaseURL)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo := postgres.New(pool)

	first := domain.User{ID: uuid.NewString(), Email: "first@example.test", PasswordHash: "hash-one"}
	created, err := repo.Create(ctx, first)
	require.NoError(t, err)
	require.Equal(t, first.Email, created.Email)
	require.False(t, created.CreatedAt.IsZero())
	loaded, err := repo.ByID(ctx, first.ID)
	require.NoError(t, err)
	require.Equal(t, first.PasswordHash, loaded.PasswordHash)
	_, err = repo.Create(ctx, domain.User{ID: uuid.NewString(), Email: first.Email, PasswordHash: "hash-two"})
	require.ErrorIs(t, err, domain.ErrConflict)
	_, err = repo.ByEmail(ctx, "missing@example.test")
	require.ErrorIs(t, err, domain.ErrNotFound)
	users, err := repo.List(ctx, nil, 10)
	require.NoError(t, err)
	require.Len(t, users, 1)
	require.Equal(t, first.ID, users[0].ID)
	require.Empty(t, users[0].PasswordHash)
}

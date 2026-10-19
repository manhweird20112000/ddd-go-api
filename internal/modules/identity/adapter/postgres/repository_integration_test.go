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
	identityapp "github.com/yourorg/go-template/internal/modules/identity/app"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	"github.com/yourorg/go-template/internal/modules/identity/migrations"
	"github.com/yourorg/go-template/internal/platform/db"
)

type integrationHasher struct{}

func (integrationHasher) Hash(context.Context, string) (string, error)         { return "test-hash", nil }
func (integrationHasher) Verify(context.Context, string, string) (bool, error) { return true, nil }

type integrationTokens struct{}

func (integrationTokens) Issue(context.Context, string) (string, int64, error) {
	return "test-access", 900, nil
}

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
	service := identityapp.NewService(repo, repo, db.NewTxManager(pool), integrationHasher{}, integrationTokens{}, repo)
	_, err = service.Register(ctx, "second@example.test", "strong-test-passphrase")
	require.NoError(t, err)
	var events int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox").Scan(&events))
	require.Equal(t, 1, events)
	_, err = pool.Exec(ctx, "DROP TABLE identity.outbox")
	require.NoError(t, err)
	_, err = service.Register(ctx, "rolled-back@example.test", "strong-test-passphrase")
	require.Error(t, err)
	_, err = repo.ByEmail(ctx, "rolled-back@example.test")
	require.ErrorIs(t, err, domain.ErrNotFound)
}

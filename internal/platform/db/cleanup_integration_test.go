//go:build integration

package db_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	identitymigrations "github.com/yourorg/go-template/internal/modules/identity/migrations"
	notificationmigrations "github.com/yourorg/go-template/internal/modules/notification/migrations"
	"github.com/yourorg/go-template/internal/platform/db"
)

func TestCleanupRetention(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pgcontainer.Run(ctx, "postgres:16", pgcontainer.BasicWaitStrategies(),
		pgcontainer.WithDatabase("cleanup_test"), pgcontainer.WithUsername("cleanup_test"),
		pgcontainer.WithPassword(uuid.NewString()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, identitymigrations.Run(ctx, url, "up"))
	require.NoError(t, notificationmigrations.Run(ctx, url, "up"))
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	for _, age := range []int{46, 1} {
		_, err = pool.Exec(ctx, `INSERT INTO identity.outbox
			(id,event_type,version,aggregate_id,occurred_at,payload,status,published_at)
			VALUES ($1,'test.event',1,$2,now(),'{}','published',now()-$3::interval)`,
			uuid.New(), uuid.New(), time.Duration(age)*24*time.Hour)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO notification.processed_events
			(event_id,consumer,processed_at) VALUES ($1,'test',now()-$2::interval)`,
			uuid.New(), time.Duration(age)*24*time.Hour)
		require.NoError(t, err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO identity.idempotency_keys
		(scope,key,request_hmac,expires_at) VALUES ('test','expired',E'\\x01',now()-interval '1 hour'),
		('test','active',E'\\x02',now()+interval '1 hour')`)
	require.NoError(t, err)
	require.NoError(t, db.CleanupOnce(ctx, pool, pool))
	for _, table := range []string{"identity.outbox", "notification.processed_events", "identity.idempotency_keys"} {
		var count int
		require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM "+table).Scan(&count))
		require.Equal(t, 1, count, table)
	}
}

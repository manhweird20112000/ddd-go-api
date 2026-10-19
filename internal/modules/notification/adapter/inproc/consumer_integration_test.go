//go:build integration

package inproc_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
	identitymigrations "github.com/yourorg/go-template/internal/modules/identity/migrations"
	"github.com/yourorg/go-template/internal/modules/notification"
	notificationmigrations "github.com/yourorg/go-template/internal/modules/notification/migrations"
	"github.com/yourorg/go-template/internal/platform/eventbus"
	"github.com/yourorg/go-template/internal/platform/outbox"
)

func TestDuplicateEventsAndConcurrentWorkers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pgcontainer.Run(ctx, "postgres:16",
		pgcontainer.BasicWaitStrategies(),
		pgcontainer.WithDatabase("notification_test"),
		pgcontainer.WithUsername("notification_test"),
		pgcontainer.WithPassword(uuid.NewString()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, identitymigrations.Run(ctx, url, "up"))
	require.NoError(t, notificationmigrations.Run(ctx, url, "up"))
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	bus := eventbus.NewInProcess()
	require.NoError(t, notification.New(pool, logger).Subscribe(bus))

	userID := uuid.NewString()
	payload, err := json.Marshal(contract.UserRegisteredV1{UserID: userID})
	require.NoError(t, err)
	event := eventbus.Event{ID: uuid.NewString(), Type: contract.UserRegisteredType,
		Version: 1, AggregateID: userID, Payload: payload}
	require.NoError(t, bus.Publish(ctx, event))
	require.NoError(t, bus.Publish(ctx, event))
	var deliveries, processed int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notification.welcome_deliveries").Scan(&deliveries))
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notification.processed_events").Scan(&processed))
	require.Equal(t, 1, deliveries)
	require.Equal(t, 1, processed)

	for range 20 {
		id := uuid.New()
		userID = uuid.NewString()
		payload, err = json.Marshal(contract.UserRegisteredV1{UserID: userID})
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO identity.outbox
			(id, event_type, version, aggregate_id, occurred_at, payload)
			VALUES ($1, $2, 1, $3, now(), $4)`, id, contract.UserRegisteredType, userID, payload)
		require.NoError(t, err)
	}
	first, err := outbox.NewRelay(pool, "identity", bus, logger)
	require.NoError(t, err)
	second, err := outbox.NewRelay(pool, "identity", bus, logger)
	require.NoError(t, err)
	var wg sync.WaitGroup
	for _, relay := range []*outbox.Relay{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, runErr := relay.RunOnce(ctx, 20)
			if runErr != nil {
				t.Errorf("relay failed: %v", runErr)
			}
		}()
	}
	wg.Wait()
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notification.welcome_deliveries").Scan(&deliveries))
	require.Equal(t, 21, deliveries)
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM notification.processed_events").Scan(&processed))
	require.Equal(t, 21, processed)
	var published int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox WHERE status = 'published'").Scan(&published))
	require.Equal(t, 20, published)
}

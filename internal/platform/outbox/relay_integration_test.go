//go:build integration

package outbox_test

import (
	"context"
	"errors"
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

	"github.com/yourorg/go-template/internal/modules/identity/migrations"
	"github.com/yourorg/go-template/internal/platform/eventbus"
	"github.com/yourorg/go-template/internal/platform/outbox"
)

type countingPublisher struct {
	mu    sync.Mutex
	count map[string]int
}

type publisherFunc func(context.Context, eventbus.Event) error

func (f publisherFunc) Publish(ctx context.Context, event eventbus.Event) error {
	return f(ctx, event)
}

func (p *countingPublisher) Publish(ctx context.Context, event eventbus.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(30 * time.Millisecond):
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.count[event.ID]++
	return nil
}

func TestConcurrentRelaysClaimEachRowOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pgcontainer.Run(ctx, "postgres:16",
		pgcontainer.BasicWaitStrategies(),
		pgcontainer.WithDatabase("outbox_test"),
		pgcontainer.WithUsername("outbox_test"),
		pgcontainer.WithPassword(uuid.NewString()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, migrations.Run(ctx, url, "up"))
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	for range 20 {
		_, err := pool.Exec(ctx, `INSERT INTO identity.outbox
			(id, event_type, version, aggregate_id, occurred_at, payload)
			VALUES ($1, 'identity.user_registered', 1, $2, now(), '{"user_id":"test"}')`,
			uuid.New(), uuid.New())
		require.NoError(t, err)
	}
	publisher := &countingPublisher{count: make(map[string]int)}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	first, err := outbox.NewRelay(pool, "identity", publisher, logger)
	require.NoError(t, err)
	second, err := outbox.NewRelay(pool, "identity", publisher, logger)
	require.NoError(t, err)
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for _, relay := range []*outbox.Relay{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			count, runErr := relay.RunOnce(ctx, 20)
			if runErr != nil {
				t.Errorf("relay: %v", runErr)
			}
			results <- count
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	total := 0
	for count := range results {
		total += count
	}
	require.Equal(t, 20, total)
	require.Len(t, publisher.count, 20)
	for _, count := range publisher.count {
		require.Equal(t, 1, count)
	}
	var published int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox WHERE status = 'published'").Scan(&published))
	require.Equal(t, 20, published)

	deadID := uuid.New()
	_, err = pool.Exec(ctx, `INSERT INTO identity.outbox
		(id, event_type, version, aggregate_id, occurred_at, payload, attempts)
		VALUES ($1, 'identity.user_registered', 1, $2, now(), '{}', 9)`, deadID, uuid.New())
	require.NoError(t, err)
	failing, err := outbox.NewRelay(pool, "identity", publisherFunc(func(context.Context, eventbus.Event) error {
		return errors.New("delivery failed")
	}), logger)
	require.NoError(t, err)
	count, err := failing.RunOnce(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	var status string
	var attempts int
	require.NoError(t, pool.QueryRow(ctx, "SELECT status, attempts FROM identity.outbox WHERE id = $1", deadID).Scan(&status, &attempts))
	require.Equal(t, "dead", status)
	require.Equal(t, 10, attempts)
}

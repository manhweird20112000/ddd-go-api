package db

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CleanupOnce deletes bounded batches after the event replay window has passed.
func CleanupOnce(ctx context.Context, identityPool, notificationPool *pgxpool.Pool) error {
	tasks := []struct {
		pool        *pgxpool.Pool
		name, query string
	}{
		{identityPool, "identity published outbox", `DELETE FROM identity.outbox WHERE id IN
			(SELECT id FROM identity.outbox WHERE status = 'published' AND published_at < now() - interval '45 days'
			 ORDER BY published_at LIMIT 500)`},
		{identityPool, "identity dead outbox", `DELETE FROM identity.outbox WHERE id IN
			(SELECT id FROM identity.outbox WHERE status = 'dead' AND occurred_at < now() - interval '90 days'
			 ORDER BY occurred_at LIMIT 500)`},
		{identityPool, "identity idempotency", `DELETE FROM identity.idempotency_keys WHERE ctid IN
			(SELECT ctid FROM identity.idempotency_keys WHERE expires_at < now() ORDER BY expires_at LIMIT 500)`},
		{notificationPool, "notification published outbox", `DELETE FROM notification.outbox WHERE id IN
			(SELECT id FROM notification.outbox WHERE status = 'published' AND published_at < now() - interval '45 days'
			 ORDER BY published_at LIMIT 500)`},
		{notificationPool, "notification dead outbox", `DELETE FROM notification.outbox WHERE id IN
			(SELECT id FROM notification.outbox WHERE status = 'dead' AND occurred_at < now() - interval '90 days'
			 ORDER BY occurred_at LIMIT 500)`},
		{notificationPool, "notification processed events", `DELETE FROM notification.processed_events WHERE ctid IN
			(SELECT ctid FROM notification.processed_events WHERE processed_at < now() - interval '45 days'
			 ORDER BY processed_at LIMIT 500)`},
	}
	for _, task := range tasks {
		if task.pool == nil {
			continue
		}
		if _, err := task.pool.Exec(ctx, task.query); err != nil {
			return fmt.Errorf("clean %s: %w", task.name, err)
		}
	}
	return nil
}

// RunCleanup performs retention work at startup and hourly until cancellation.
func RunCleanup(ctx context.Context, identityPool, notificationPool *pgxpool.Pool, logger *slog.Logger) error {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		attemptCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := CleanupOnce(attemptCtx, identityPool, notificationPool)
		cancel()
		if err != nil && ctx.Err() == nil {
			logger.ErrorContext(ctx, "cleanup failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

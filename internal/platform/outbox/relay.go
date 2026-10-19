// Package outbox relays committed events from module-owned PostgreSQL tables.
package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/platform/eventbus"
)

const maxAttempts = 10

// Relay claims one module's events and sends them to a bus.
type Relay struct {
	pool    *pgxpool.Pool
	bus     eventbus.Publisher
	logger  *slog.Logger
	selectQ string
	updateQ string
}

// NewRelay accepts only known module schemas, preventing SQL identifier injection.
func NewRelay(pool *pgxpool.Pool, schema string, bus eventbus.Publisher, logger *slog.Logger) (*Relay, error) {
	if pool == nil || bus == nil || logger == nil {
		return nil, errors.New("outbox relay needs pool, bus, and logger")
	}
	var table string
	switch schema {
	case "identity":
		table = "identity.outbox"
	case "notification":
		table = "notification.outbox"
	default:
		return nil, fmt.Errorf("unsupported outbox schema %q", schema)
	}
	return &Relay{
		pool: pool, bus: bus, logger: logger,
		selectQ: "SELECT id, event_type, version, aggregate_id, occurred_at, request_id, trace_id, traceparent, payload, attempts FROM " + table +
			" WHERE status = 'pending' AND next_attempt_at <= now() ORDER BY next_attempt_at, occurred_at, id LIMIT $1 FOR UPDATE SKIP LOCKED",
		updateQ: "UPDATE " + table + " SET status = $2, attempts = $3, next_attempt_at = $4, published_at = $5, last_error = $6 WHERE id = $1",
	}, nil
}

// Run polls until cancellation, allowing multiple replicas to relay concurrently.
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if _, err := r.RunOnce(ctx, 50); err != nil && ctx.Err() == nil {
			r.logger.ErrorContext(ctx, "outbox relay failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// RunOnce claims and processes at most limit due events in one transaction.
func (r *Relay) RunOnce(ctx context.Context, limit int) (count int, err error) {
	if limit < 1 || limit > 100 {
		return 0, errors.New("outbox batch size must be 1..100")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin outbox claim: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			err = errors.Join(err, fmt.Errorf("rollback outbox claim: %w", rollbackErr))
		}
	}()
	rows, err := tx.Query(ctx, r.selectQ, limit)
	if err != nil {
		return 0, fmt.Errorf("claim outbox events: %w", err)
	}
	type claimed struct {
		event    eventbus.Event
		attempts int
	}
	batch := make([]claimed, 0, limit)
	for rows.Next() {
		var item claimed
		var id, aggregateID uuid.UUID
		if err := rows.Scan(&id, &item.event.Type, &item.event.Version, &aggregateID,
			&item.event.OccurredAt, &item.event.RequestID, &item.event.TraceID,
			&item.event.Traceparent, &item.event.Payload, &item.attempts); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan outbox event: %w", err)
		}
		item.event.ID = id.String()
		item.event.AggregateID = aggregateID.String()
		batch = append(batch, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate outbox events: %w", err)
	}
	rows.Close()
	for _, item := range batch {
		status := "published"
		attempts := item.attempts
		var publishedAt *time.Time
		nextAttempt := time.Now().UTC()
		lastError := ""
		publishErr := r.bus.Publish(ctx, item.event)
		if errors.Is(publishErr, eventbus.ErrNoSubscriber) {
			continue
		}
		if publishErr != nil {
			attempts++
			status = "pending"
			if attempts >= maxAttempts {
				status = "dead"
				r.logger.ErrorContext(ctx, "outbox event dead", "event_id", item.event.ID, "event_type", item.event.Type)
			}
			nextAttempt = nextAttempt.Add(retryDelay(attempts))
			lastError = publishErr.Error()
			if len(lastError) > 512 {
				lastError = lastError[:512]
			}
		} else {
			publishedAt = &nextAttempt
		}
		if _, err := tx.Exec(ctx, r.updateQ, item.event.ID, status, attempts, nextAttempt, publishedAt, lastError); err != nil {
			return count, fmt.Errorf("update outbox event: %w", err)
		}
		count++
	}
	if err := tx.Commit(ctx); err != nil {
		return count, fmt.Errorf("commit outbox batch: %w", err)
	}
	return count, nil
}

func retryDelay(attempts int) time.Duration {
	if attempts > 8 {
		attempts = 8
	}
	return time.Duration(1<<attempts) * time.Second
}

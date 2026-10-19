package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/yourorg/go-template/internal/modules/identity/adapter/postgres/sqlcgen"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// RecordUserRegistered persists a versioned event in the active transaction.
func (r *Repository) RecordUserRegistered(ctx context.Context, userID string) error {
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("new outbox ID: %w", err)
	}
	aggregateID, err := parseID(userID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(contract.UserRegisteredV1{UserID: userID})
	if err != nil {
		return fmt.Errorf("encode UserRegistered: %w", err)
	}
	span := trace.SpanContextFromContext(ctx)
	traceID := ""
	if span.IsValid() {
		traceID = span.TraceID().String()
	}
	carrier := propagation.MapCarrier{}
	propagation.TraceContext{}.Inject(ctx, carrier)
	if err := r.queries(ctx).InsertOutboxEvent(ctx, sqlcgen.InsertOutboxEventParams{
		ID:          pgtype.UUID{Bytes: [16]byte(id), Valid: true},
		EventType:   contract.UserRegisteredType,
		Version:     1,
		AggregateID: aggregateID,
		OccurredAt:  pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
		RequestID:   httpx.RequestID(ctx),
		TraceID:     traceID,
		Traceparent: carrier.Get("traceparent"),
		Payload:     payload,
	}); err != nil {
		return fmt.Errorf("insert UserRegistered outbox event: %w", err)
	}
	return nil
}

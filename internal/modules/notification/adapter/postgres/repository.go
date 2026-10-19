// Package postgres implements notification persistence with sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/modules/notification/adapter/postgres/sqlcgen"
	"github.com/yourorg/go-template/internal/modules/notification/domain"
	"github.com/yourorg/go-template/internal/platform/db"
)

// Repository stores notification consumer state in its own schema.
type Repository struct{ pool *pgxpool.Pool }

// New creates a transaction-aware notification repository.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.FromContext(ctx, r.pool))
}

// MarkProcessed inserts an idempotency marker and reports whether it was new.
func (r *Repository) MarkProcessed(ctx context.Context, eventID, consumer string) (bool, error) {
	id, err := parseID(eventID)
	if err != nil {
		return false, err
	}
	_, err = r.queries(ctx).MarkEventProcessed(ctx, sqlcgen.MarkEventProcessedParams{EventID: id, Consumer: consumer})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("mark event processed: %w", err)
	}
	return true, nil
}

// SaveWelcome records the notification database effect.
func (r *Repository) SaveWelcome(ctx context.Context, eventID, userID, recipient string) error {
	id, err := parseID(eventID)
	if err != nil {
		return err
	}
	user, err := parseID(userID)
	if err != nil {
		return err
	}
	if err := r.queries(ctx).InsertWelcomeDelivery(ctx, sqlcgen.InsertWelcomeDeliveryParams{
		EventID: id, UserID: user, Recipient: recipient,
	}); err != nil {
		return fmt.Errorf("insert welcome delivery: %w", err)
	}
	return nil
}

func parseID(raw string) (pgtype.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("parse notification ID: %w", domain.ErrInvalidEvent)
	}
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}, nil
}

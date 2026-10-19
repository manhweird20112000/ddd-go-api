// Package postgres persists identity aggregates through sqlc queries.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/modules/identity/adapter/postgres/sqlcgen"
	"github.com/yourorg/go-template/internal/modules/identity/app"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	"github.com/yourorg/go-template/internal/platform/db"
)

// Repository implements identity user and refresh-session persistence.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a repository that resolves active transactions from context.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) queries(ctx context.Context) *sqlcgen.Queries {
	return sqlcgen.New(db.FromContext(ctx, r.pool))
}

// Create inserts a user and maps unique-email violations to a domain conflict.
func (r *Repository) Create(ctx context.Context, user domain.User) (domain.User, error) {
	id, err := parseID(user.ID)
	if err != nil {
		return domain.User{}, err
	}
	row, err := r.queries(ctx).CreateUser(ctx, sqlcgen.CreateUserParams{
		ID: id, Email: user.Email, PasswordHash: user.PasswordHash,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return domain.User{}, fmt.Errorf("create user: %w", domain.ErrConflict)
		}
		return domain.User{}, fmt.Errorf("create user: %w", err)
	}
	return mapUser(row), nil
}

// ByEmail returns a user by canonical email.
func (r *Repository) ByEmail(ctx context.Context, email string) (domain.User, error) {
	row, err := r.queries(ctx).FindUserByEmail(ctx, email)
	if err != nil {
		return domain.User{}, mapReadError("find user by email", err)
	}
	return mapUser(row), nil
}

// ByID returns a user by ID.
func (r *Repository) ByID(ctx context.Context, rawID string) (domain.User, error) {
	id, err := parseID(rawID)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", domain.ErrNotFound)
	}
	row, err := r.queries(ctx).GetUser(ctx, id)
	if err != nil {
		return domain.User{}, mapReadError("get user", err)
	}
	return mapUser(row), nil
}

// List returns a bounded page in stable creation order.
func (r *Repository) List(ctx context.Context, after *app.Cursor, limit int) ([]domain.User, error) {
	if limit < 1 || limit > 101 {
		return nil, fmt.Errorf("list users limit: %w", domain.ErrValidation)
	}
	users := make([]domain.User, 0, limit)
	if after == nil {
		rows, err := r.queries(ctx).ListFirstUsers(ctx, int32(limit))
		if err != nil {
			return nil, fmt.Errorf("list first users: %w", err)
		}
		for _, row := range rows {
			users = append(users, domain.User{ID: uuid.UUID(row.ID.Bytes).String(), CreatedAt: row.CreatedAt.Time})
		}
		return users, nil
	}
	id, err := parseID(after.ID)
	if err != nil {
		return nil, fmt.Errorf("list users cursor: %w", domain.ErrValidation)
	}
	rows, err := r.queries(ctx).ListUsersAfter(ctx, sqlcgen.ListUsersAfterParams{
		AfterAt: pgtype.Timestamptz{Time: after.CreatedAt, Valid: true},
		AfterID: id, PageLimit: int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("list users after cursor: %w", err)
	}
	for _, row := range rows {
		users = append(users, domain.User{ID: uuid.UUID(row.ID.Bytes).String(), CreatedAt: row.CreatedAt.Time})
	}
	return users, nil
}

// Insert persists a hashed refresh token.
func (r *Repository) Insert(ctx context.Context, token domain.RefreshToken) error {
	id, err := parseID(token.ID)
	if err != nil {
		return err
	}
	userID, err := parseID(token.UserID)
	if err != nil {
		return err
	}
	familyID, err := parseID(token.FamilyID)
	if err != nil {
		return err
	}
	if err := r.queries(ctx).InsertRefreshToken(ctx, sqlcgen.InsertRefreshTokenParams{
		ID: id, UserID: userID, FamilyID: familyID, TokenHash: token.Hash,
		ExpiresAt: pgtype.Timestamptz{Time: token.ExpiresAt, Valid: true},
	}); err != nil {
		return fmt.Errorf("insert refresh token: %w", err)
	}
	return nil
}

// LockByHash loads and locks a refresh token for rotation.
func (r *Repository) LockByHash(ctx context.Context, hash []byte) (domain.RefreshToken, error) {
	row, err := r.queries(ctx).GetRefreshTokenForUpdate(ctx, hash)
	if err != nil {
		return domain.RefreshToken{}, mapReadError("get refresh token", err)
	}
	return domain.RefreshToken{
		ID: uuid.UUID(row.ID.Bytes).String(), UserID: uuid.UUID(row.UserID.Bytes).String(),
		FamilyID: uuid.UUID(row.FamilyID.Bytes).String(), Hash: row.TokenHash,
		ExpiresAt: row.ExpiresAt.Time, UsedAt: optionalTime(row.UsedAt), RevokedAt: optionalTime(row.RevokedAt),
	}, nil
}

// MarkUsed marks a refresh token as rotated.
func (r *Repository) MarkUsed(ctx context.Context, rawID string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	if err := r.queries(ctx).MarkRefreshTokenUsed(ctx, id); err != nil {
		return fmt.Errorf("mark refresh token used: %w", err)
	}
	return nil
}

// RevokeFamily invalidates every refresh token in a compromised family.
func (r *Repository) RevokeFamily(ctx context.Context, rawFamilyID string) error {
	id, err := parseID(rawFamilyID)
	if err != nil {
		return err
	}
	if err := r.queries(ctx).RevokeRefreshFamily(ctx, id); err != nil {
		return fmt.Errorf("revoke refresh family: %w", err)
	}
	return nil
}

func parseID(raw string) (pgtype.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid UUID: %w", domain.ErrValidation)
	}
	return pgtype.UUID{Bytes: [16]byte(id), Valid: true}, nil
}

func mapUser(row sqlcgen.IdentityUser) domain.User {
	return domain.User{
		ID: uuid.UUID(row.ID.Bytes).String(), Email: row.Email,
		PasswordHash: row.PasswordHash, CreatedAt: row.CreatedAt.Time,
	}
}

func mapReadError(operation string, err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", operation, domain.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func optionalTime(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

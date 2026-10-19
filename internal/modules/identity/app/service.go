// Package app implements identity commands and queries through small ports.
package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

// Cursor identifies the last user in a page ordered by creation time and ID.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

// UserRepository stores and queries identity users.
type UserRepository interface {
	Create(context.Context, domain.User) (domain.User, error)
	ByEmail(context.Context, string) (domain.User, error)
	ByID(context.Context, string) (domain.User, error)
	List(context.Context, *Cursor, int) ([]domain.User, error)
}

// SessionRepository stores and rotates hashed refresh tokens.
type SessionRepository interface {
	Insert(context.Context, domain.RefreshToken) error
	LockByHash(context.Context, []byte) (domain.RefreshToken, error)
	MarkUsed(context.Context, string) error
	RevokeFamily(context.Context, string) error
}

// UnitOfWork groups database writes into one transaction.
type UnitOfWork interface {
	WithinTx(context.Context, func(context.Context) error) error
}

// Hasher hashes and verifies passwords.
type Hasher interface {
	Hash(context.Context, string) (string, error)
	Verify(context.Context, string, string) (bool, error)
}

// AccessTokens issues signed access tokens.
type AccessTokens interface {
	Issue(context.Context, string) (string, int64, error)
}

// TokenPair is returned after login and refresh rotation.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int64
}

// Service implements identity commands and queries.
type Service struct {
	users    UserRepository
	sessions SessionRepository
	tx       UnitOfWork
	hasher   Hasher
	tokens   AccessTokens
}

// NewService builds identity use cases from injected ports.
func NewService(users UserRepository, sessions SessionRepository, tx UnitOfWork, hasher Hasher, tokens AccessTokens) *Service {
	return &Service{users: users, sessions: sessions, tx: tx, hasher: hasher, tokens: tokens}
}

// Register creates a user in one transaction after validating and hashing input.
func (s *Service) Register(ctx context.Context, rawEmail, password string) (domain.User, error) {
	email, err := domain.NormalizeEmail(rawEmail)
	if err != nil {
		return domain.User{}, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return domain.User{}, err
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}
	id, err := newID()
	if err != nil {
		return domain.User{}, err
	}
	var created domain.User
	err = s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		var createErr error
		created, createErr = s.users.Create(txCtx, domain.User{ID: id, Email: email, PasswordHash: hash})
		return createErr
	})
	if err != nil {
		return domain.User{}, fmt.Errorf("register user: %w", err)
	}
	return created, nil
}

// Login verifies credentials and creates a hashed refresh-token session.
func (s *Service) Login(ctx context.Context, rawEmail, password string) (TokenPair, error) {
	email, err := domain.NormalizeEmail(rawEmail)
	if err != nil {
		return TokenPair{}, fmt.Errorf("login: %w", domain.ErrUnauthorized)
	}
	user, err := s.users.ByEmail(ctx, email)
	if errors.Is(err, domain.ErrNotFound) {
		return TokenPair{}, fmt.Errorf("login: %w", domain.ErrUnauthorized)
	}
	if err != nil {
		return TokenPair{}, fmt.Errorf("find user for login: %w", err)
	}
	valid, err := s.hasher.Verify(ctx, user.PasswordHash, password)
	if err != nil {
		return TokenPair{}, fmt.Errorf("verify password: %w", err)
	}
	if !valid {
		return TokenPair{}, fmt.Errorf("login: %w", domain.ErrUnauthorized)
	}
	familyID, err := newID()
	if err != nil {
		return TokenPair{}, err
	}
	return s.createSession(ctx, user.ID, familyID)
}

// Refresh rotates a refresh token and revokes its entire family on reuse.
func (s *Service) Refresh(ctx context.Context, raw string) (TokenPair, error) {
	if raw == "" || len(raw) > 512 {
		return TokenPair{}, fmt.Errorf("refresh: %w", domain.ErrUnauthorized)
	}
	hash := sha256.Sum256([]byte(raw))
	var result TokenPair
	var reused bool
	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		stored, err := s.sessions.LockByHash(txCtx, hash[:])
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ErrUnauthorized
		}
		if err != nil {
			return err
		}
		if stored.UsedAt != nil {
			if err := s.sessions.RevokeFamily(txCtx, stored.FamilyID); err != nil {
				return err
			}
			reused = true
			return nil
		}
		if stored.RevokedAt != nil || !stored.ExpiresAt.After(time.Now()) {
			return domain.ErrUnauthorized
		}
		result, err = s.newTokenPair(txCtx, stored.UserID, stored.FamilyID)
		if err != nil {
			return err
		}
		return s.sessions.MarkUsed(txCtx, stored.ID)
	})
	if err != nil {
		if errors.Is(err, domain.ErrUnauthorized) {
			return TokenPair{}, fmt.Errorf("refresh: %w", domain.ErrUnauthorized)
		}
		return TokenPair{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	if reused {
		return TokenPair{}, fmt.Errorf("refresh reuse: %w", domain.ErrUnauthorized)
	}
	return result, nil
}

// Me returns the user identified by a verified access token.
func (s *Service) Me(ctx context.Context, userID string) (domain.User, error) {
	user, err := s.users.ByID(ctx, userID)
	if err != nil {
		return domain.User{}, fmt.Errorf("get current user: %w", err)
	}
	return user, nil
}

// ListUsers returns one page and a cursor for the next page, if present.
func (s *Service) ListUsers(ctx context.Context, after *Cursor, limit int) ([]domain.User, *Cursor, error) {
	if limit < 1 || limit > 100 || (after != nil && (after.ID == "" || after.CreatedAt.IsZero())) {
		return nil, nil, fmt.Errorf("list users: %w", domain.ErrValidation)
	}
	users, err := s.users.List(ctx, after, limit+1)
	if err != nil {
		return nil, nil, fmt.Errorf("list users: %w", err)
	}
	if len(users) <= limit {
		return users, nil, nil
	}
	users = users[:limit]
	last := users[len(users)-1]
	return users, &Cursor{CreatedAt: last.CreatedAt, ID: last.ID}, nil
}

func (s *Service) createSession(ctx context.Context, userID, familyID string) (TokenPair, error) {
	var pair TokenPair
	err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		var createErr error
		pair, createErr = s.newTokenPair(txCtx, userID, familyID)
		return createErr
	})
	if err != nil {
		return TokenPair{}, fmt.Errorf("create session: %w", err)
	}
	return pair, nil
}

func (s *Service) newTokenPair(ctx context.Context, userID, familyID string) (TokenPair, error) {
	access, expiresIn, err := s.tokens.Issue(ctx, userID)
	if err != nil {
		return TokenPair{}, fmt.Errorf("issue access token: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return TokenPair{}, err
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return TokenPair{}, fmt.Errorf("generate refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(secret[:])
	hash := sha256.Sum256([]byte(raw))
	id, err := newID()
	if err != nil {
		return TokenPair{}, err
	}
	if err := s.sessions.Insert(ctx, domain.RefreshToken{
		ID: id, UserID: userID, FamilyID: familyID, Hash: hash[:], ExpiresAt: time.Now().Add(30 * 24 * time.Hour),
	}); err != nil {
		return TokenPair{}, fmt.Errorf("store refresh token: %w", err)
	}
	return TokenPair{AccessToken: access, RefreshToken: raw, ExpiresIn: expiresIn}, nil
}

func newID() (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate UUIDv7: %w", err)
	}
	return id.String(), nil
}

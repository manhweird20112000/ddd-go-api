// Package domain contains identity business values and errors without adapters.
package domain

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

var (
	// ErrNotFound means a requested identity record does not exist.
	ErrNotFound = errors.New("identity record not found")
	// ErrConflict means an identity value already exists.
	ErrConflict = errors.New("identity conflict")
	// ErrUnauthorized means credentials or a token are invalid.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrValidation means input violates identity rules.
	ErrValidation = errors.New("identity validation failed")
)

// User is the identity module's account entity.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
}

// RefreshToken contains persisted rotation state, never the raw token.
type RefreshToken struct {
	ID        string
	UserID    string
	FamilyID  string
	Hash      []byte
	ExpiresAt time.Time
	UsedAt    *time.Time
	RevokedAt *time.Time
}

// NormalizeEmail validates and canonicalizes an account email address.
func NormalizeEmail(raw string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(raw))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return "", fmt.Errorf("email: %w", ErrValidation)
	}
	return email, nil
}

// ValidatePassword bounds password size before an expensive hash operation.
func ValidatePassword(password string) error {
	if len(password) < 12 || len(password) > 1024 {
		return fmt.Errorf("password must contain 12 to 1024 bytes: %w", ErrValidation)
	}
	return nil
}

// Package domain defines notification's transport-independent state and errors.
package domain

import "errors"

// ErrInvalidEvent indicates a malformed message that must not be acknowledged.
var ErrInvalidEvent = errors.New("invalid notification event")

// RegisteredUser identifies a source event and user without importing identity.
type RegisteredUser struct {
	EventID string
	UserID  string
}

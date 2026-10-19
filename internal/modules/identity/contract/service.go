package contract

import (
	"context"
	"time"
)

// User is the internal DTO shared with consumers of the identity contract.
type User struct {
	ID        string
	Email     string
	CreatedAt time.Time
}

// Service is the stable synchronous identity contract.
type Service interface {
	GetUser(context.Context, string) (User, error)
}

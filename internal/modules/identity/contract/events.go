// Package contract contains identity's public module contracts.
package contract

// UserRegisteredV1 is the payload published after a user is created.
type UserRegisteredV1 struct {
	UserID string `json:"user_id"`
}

// UserRegisteredType is the stable event name consumed by other modules.
const UserRegisteredType = "identity.user_registered"

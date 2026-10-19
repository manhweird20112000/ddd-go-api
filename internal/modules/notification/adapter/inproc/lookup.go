package inproc

import (
	"context"
	"fmt"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
)

// Lookup adapts the public identity contract to notification's lookup port.
type Lookup struct{ identity contract.Service }

// NewLookup builds a local identity contract client.
func NewLookup(identity contract.Service) *Lookup { return &Lookup{identity: identity} }

// Email returns the current email for the referenced identity user.
func (l *Lookup) Email(ctx context.Context, userID string) (string, error) {
	user, err := l.identity.GetUser(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("identity GetUser: %w", err)
	}
	return user.Email, nil
}

package app

import (
	"context"
	"fmt"

	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

// Queries exposes identity's read-only application use cases.
type Queries struct{ users UserRepository }

// NewQueries builds identity queries without signing credentials.
func NewQueries(users UserRepository) *Queries { return &Queries{users: users} }

// GetUser loads one user for the module contract.
func (q *Queries) GetUser(ctx context.Context, userID string) (domain.User, error) {
	user, err := q.users.ByID(ctx, userID)
	if err != nil {
		return domain.User{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

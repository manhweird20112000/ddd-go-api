// Package inproc implements identity's public contract in the same process.
package inproc

import (
	"context"
	"fmt"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

// Query is the identity application query consumed by this adapter.
type Query interface {
	GetUser(context.Context, string) (domain.User, error)
}

// Service adapts identity queries to the public module contract.
type Service struct{ query Query }

// New builds a local identity contract adapter.
func New(query Query) *Service { return &Service{query: query} }

// GetUser returns a user DTO without its password hash.
func (s *Service) GetUser(ctx context.Context, userID string) (contract.User, error) {
	user, err := s.query.GetUser(ctx, userID)
	if err != nil {
		return contract.User{}, fmt.Errorf("identity local GetUser: %w", err)
	}
	return contract.User{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt}, nil
}

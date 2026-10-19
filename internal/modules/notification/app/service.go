// Package app handles notification events through small persistence and sender ports.
package app

import (
	"context"
	"fmt"

	"github.com/yourorg/go-template/internal/modules/notification/domain"
)

const consumerName = "notification.welcome"

// Repository records processed events and welcome deliveries.
type Repository interface {
	MarkProcessed(context.Context, string, string) (bool, error)
	SaveWelcome(context.Context, string, string, string) error
}

// UnitOfWork keeps the processed marker and delivery in one transaction.
type UnitOfWork interface {
	WithinTx(context.Context, func(context.Context) error) error
}

// Sender performs the example welcome action.
type Sender interface {
	SendWelcome(context.Context, string) error
}

// Service consumes registration events with an idempotent database effect.
type Service struct {
	repo   Repository
	tx     UnitOfWork
	sender Sender
}

// NewService connects the notification use case to its adapters.
func NewService(repo Repository, tx UnitOfWork, sender Sender) *Service {
	return &Service{repo: repo, tx: tx, sender: sender}
}

// HandleRegistered records and sends one welcome for each source event.
func (s *Service) HandleRegistered(ctx context.Context, event domain.RegisteredUser) error {
	if event.EventID == "" || event.UserID == "" {
		return fmt.Errorf("handle registration: %w", domain.ErrInvalidEvent)
	}
	if err := s.tx.WithinTx(ctx, func(txCtx context.Context) error {
		fresh, err := s.repo.MarkProcessed(txCtx, event.EventID, consumerName)
		if err != nil || !fresh {
			return err
		}
		if err := s.sender.SendWelcome(txCtx, event.UserID); err != nil {
			return fmt.Errorf("send welcome: %w", err)
		}
		if err := s.repo.SaveWelcome(txCtx, event.EventID, event.UserID, event.UserID); err != nil {
			return fmt.Errorf("save welcome delivery: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("process registration event: %w", err)
	}
	return nil
}

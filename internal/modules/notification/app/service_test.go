package app

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/go-template/internal/modules/notification/domain"
)

type fakeRepo struct {
	seen     bool
	welcomes int
}

func (r *fakeRepo) MarkProcessed(context.Context, string, string) (bool, error) {
	if r.seen {
		return false, nil
	}
	r.seen = true
	return true, nil
}

func (r *fakeRepo) SaveWelcome(context.Context, string, string, string) error {
	r.welcomes++
	return nil
}

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeSender struct{ calls int }

func (s *fakeSender) SendWelcome(context.Context, string) error {
	s.calls++
	return nil
}

func TestHandleRegisteredOnce(t *testing.T) {
	repo := &fakeRepo{}
	sender := &fakeSender{}
	service := NewService(repo, fakeTx{}, sender)
	event := domain.RegisteredUser{EventID: "event-1", UserID: "user-1"}
	for range 2 {
		if err := service.HandleRegistered(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if sender.calls != 1 || repo.welcomes != 1 {
		t.Fatalf("expected one welcome, got sends=%d rows=%d", sender.calls, repo.welcomes)
	}
	if err := service.HandleRegistered(context.Background(), domain.RegisteredUser{}); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("expected invalid event, got %v", err)
	}
}

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/yourorg/go-template/internal/modules/notification/domain"
)

type fakeRepo struct {
	seen             bool
	welcomes         int
	markErr, saveErr error
}

func (r *fakeRepo) MarkProcessed(context.Context, string, string) (bool, error) {
	if r.markErr != nil {
		return false, r.markErr
	}
	if r.seen {
		return false, nil
	}
	r.seen = true
	return true, nil
}

func (r *fakeRepo) SaveWelcome(context.Context, string, string, string) error {
	r.welcomes++
	return r.saveErr
}

type fakeTx struct{}

func (fakeTx) WithinTx(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }

type fakeSender struct {
	calls int
	err   error
}

func (s *fakeSender) SendWelcome(context.Context, string) error {
	s.calls++
	return s.err
}

type fakeUsers struct {
	calls    int
	email    string
	err      error
	explicit bool
}

func (u *fakeUsers) Email(context.Context, string) (string, error) {
	u.calls++
	if !u.explicit && u.err == nil {
		return "recipient@example.test", nil
	}
	return u.email, u.err
}

func TestHandleRegisteredFailurePaths(t *testing.T) {
	for _, test := range []struct {
		name   string
		repo   *fakeRepo
		sender *fakeSender
		users  *fakeUsers
	}{
		{"marker failure", &fakeRepo{markErr: errors.New("marker failed")}, &fakeSender{}, &fakeUsers{}},
		{"lookup failure", &fakeRepo{}, &fakeSender{}, &fakeUsers{err: errors.New("lookup failed")}},
		{"empty recipient", &fakeRepo{}, &fakeSender{}, &fakeUsers{explicit: true}},
		{"send failure", &fakeRepo{}, &fakeSender{err: errors.New("send failed")}, &fakeUsers{}},
		{"save failure", &fakeRepo{saveErr: errors.New("save failed")}, &fakeSender{}, &fakeUsers{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := NewService(test.repo, fakeTx{}, test.sender, test.users).HandleRegistered(context.Background(),
				domain.RegisteredUser{EventID: "event-1", UserID: "user-1"})
			if err == nil {
				t.Fatal("expected event handling failure")
			}
		})
	}
}

func TestHandleRegisteredOnce(t *testing.T) {
	repo := &fakeRepo{}
	sender := &fakeSender{}
	users := &fakeUsers{}
	service := NewService(repo, fakeTx{}, sender, users)
	event := domain.RegisteredUser{EventID: "event-1", UserID: "user-1"}
	for range 2 {
		if err := service.HandleRegistered(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	if sender.calls != 1 || repo.welcomes != 1 || users.calls != 1 {
		t.Fatalf("expected one welcome, got sends=%d rows=%d", sender.calls, repo.welcomes)
	}
	if err := service.HandleRegistered(context.Background(), domain.RegisteredUser{}); !errors.Is(err, domain.ErrInvalidEvent) {
		t.Fatalf("expected invalid event, got %v", err)
	}
}

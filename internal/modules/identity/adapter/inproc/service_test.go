package inproc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

type fakeQuery struct {
	user domain.User
	err  error
}

func (f fakeQuery) GetUser(context.Context, string) (domain.User, error) {
	return f.user, f.err
}

func TestGetUserProjectsPublicDTO(t *testing.T) {
	created := time.Now().UTC()
	service := New(fakeQuery{user: domain.User{
		ID: "user-1", Email: "test@example.test", CreatedAt: created, PasswordHash: "secret-hash",
	}})
	user, err := service.GetUser(context.Background(), "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if user.ID != "user-1" || user.Email != "test@example.test" || !user.CreatedAt.Equal(created) {
		t.Fatalf("unexpected contract DTO: %+v", user)
	}
	missing := errors.New("missing")
	_, err = New(fakeQuery{err: missing}).GetUser(context.Background(), "user-2")
	if !errors.Is(err, missing) {
		t.Fatalf("expected wrapped query error, got %v", err)
	}
}

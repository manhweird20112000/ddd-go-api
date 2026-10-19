package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

type fakeUsers struct {
	created domain.User
	lookup  domain.User
}

func (f *fakeUsers) Create(_ context.Context, user domain.User) (domain.User, error) {
	f.created = user
	user.CreatedAt = time.Now()
	return user, nil
}
func (f *fakeUsers) ByEmail(context.Context, string) (domain.User, error) {
	return f.lookup, nil
}
func (f *fakeUsers) ByID(context.Context, string) (domain.User, error) {
	return f.lookup, nil
}
func (f *fakeUsers) List(context.Context, *Cursor, int) ([]domain.User, error) {
	return nil, nil
}

type fakeSessions struct {
	token   domain.RefreshToken
	revoked bool
}

func (f *fakeSessions) Insert(_ context.Context, token domain.RefreshToken) error {
	f.token = token
	return nil
}
func (f *fakeSessions) LockByHash(context.Context, []byte) (domain.RefreshToken, error) {
	return f.token, nil
}
func (f *fakeSessions) MarkUsed(context.Context, string) error { return nil }
func (f *fakeSessions) RevokeFamily(context.Context, string) error {
	f.revoked = true
	return nil
}

type fakeUnitOfWork struct{}

func (fakeUnitOfWork) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeHasher struct{}

func (fakeHasher) Hash(context.Context, string) (string, error) { return "stored-hash", nil }
func (fakeHasher) Verify(_ context.Context, _, password string) (bool, error) {
	return password == "correct-password", nil
}

type fakeAccessTokens struct{}

func (fakeAccessTokens) Issue(context.Context, string) (string, int64, error) {
	return "signed-access-token", 900, nil
}

func TestRegisterNormalizesAndHashes(t *testing.T) {
	users := &fakeUsers{}
	service := NewService(users, &fakeSessions{}, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{})
	user, err := service.Register(context.Background(), " Alice@Example.COM ", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "alice@example.com" || users.created.PasswordHash != "stored-hash" || user.ID == "" {
		t.Fatalf("unexpected registered user: %+v", user)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	users := &fakeUsers{lookup: domain.User{ID: "8f7e8db2-c925-4fce-889b-1901c90210bb", Email: "a@example.com", PasswordHash: "stored-hash"}}
	service := NewService(users, &fakeSessions{}, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{})
	_, err := service.Login(context.Background(), "a@example.com", "wrong-password")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("expected unauthorized, got %v", err)
	}
}

func TestRefreshReuseRevokesFamily(t *testing.T) {
	used := time.Now()
	hash := sha256.Sum256([]byte("old-raw-token"))
	sessions := &fakeSessions{token: domain.RefreshToken{
		ID: "2fd4f42b-ff67-4749-a714-4ca88660ca14", UserID: "8f7e8db2-c925-4fce-889b-1901c90210bb",
		FamilyID: "8f7e8db2-c925-4fce-889b-1901c90210bb", Hash: hash[:],
		ExpiresAt: time.Now().Add(time.Hour), UsedAt: &used,
	}}
	service := NewService(&fakeUsers{}, sessions, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{})
	_, err := service.Refresh(context.Background(), "old-raw-token")
	if !errors.Is(err, domain.ErrUnauthorized) || !sessions.revoked {
		t.Fatalf("expected family revocation and unauthorized, got revoked=%v error=%v", sessions.revoked, err)
	}
}

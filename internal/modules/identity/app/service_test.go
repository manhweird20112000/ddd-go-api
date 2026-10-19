package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

type fakeUsers struct {
	created                             domain.User
	lookup                              domain.User
	listed                              []domain.User
	createErr, emailErr, idErr, listErr error
	listLimit                           int
}

func (f *fakeUsers) Create(_ context.Context, user domain.User) (domain.User, error) {
	f.created = user
	if f.createErr != nil {
		return domain.User{}, f.createErr
	}
	user.CreatedAt = time.Now()
	return user, nil
}
func (f *fakeUsers) ByEmail(context.Context, string) (domain.User, error) {
	return f.lookup, f.emailErr
}
func (f *fakeUsers) ByID(context.Context, string) (domain.User, error) {
	return f.lookup, f.idErr
}
func (f *fakeUsers) List(_ context.Context, _ *Cursor, limit int) ([]domain.User, error) {
	f.listLimit = limit
	return f.listed, f.listErr
}

type fakeSessions struct {
	token                                  domain.RefreshToken
	revoked                                bool
	marked                                 bool
	insertErr, lockErr, markErr, revokeErr error
}

func (f *fakeSessions) Insert(_ context.Context, token domain.RefreshToken) error {
	f.token = token
	return f.insertErr
}
func (f *fakeSessions) LockByHash(context.Context, []byte) (domain.RefreshToken, error) {
	return f.token, f.lockErr
}
func (f *fakeSessions) MarkUsed(context.Context, string) error { f.marked = true; return f.markErr }
func (f *fakeSessions) RevokeFamily(context.Context, string) error {
	f.revoked = true
	return f.revokeErr
}

type fakeUnitOfWork struct{}

func (fakeUnitOfWork) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeHasher struct{ hashErr, verifyErr error }

func (h fakeHasher) Hash(context.Context, string) (string, error) { return "stored-hash", h.hashErr }
func (h fakeHasher) Verify(_ context.Context, _, password string) (bool, error) {
	return password == "correct-password", h.verifyErr
}

type fakeAccessTokens struct{ err error }

func (f fakeAccessTokens) Issue(context.Context, string) (string, int64, error) {
	return "signed-access-token", 900, f.err
}

type fakeEvents struct {
	userID string
	err    error
}

func (f *fakeEvents) RecordUserRegistered(_ context.Context, userID string) error {
	f.userID = userID
	return f.err
}

func TestRegisterNormalizesAndHashes(t *testing.T) {
	users := &fakeUsers{}
	events := &fakeEvents{}
	service := NewService(users, &fakeSessions{}, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, events)
	user, err := service.Register(context.Background(), " Alice@Example.COM ", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "alice@example.com" || users.created.PasswordHash != "stored-hash" || user.ID == "" || events.userID != user.ID {
		t.Fatalf("unexpected registered user: %+v", user)
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	users := &fakeUsers{lookup: domain.User{ID: "8f7e8db2-c925-4fce-889b-1901c90210bb", Email: "a@example.com", PasswordHash: "stored-hash"}}
	service := NewService(users, &fakeSessions{}, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, &fakeEvents{})
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
	service := NewService(&fakeUsers{}, sessions, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, &fakeEvents{})
	_, err := service.Refresh(context.Background(), "old-raw-token")
	if !errors.Is(err, domain.ErrUnauthorized) || !sessions.revoked {
		t.Fatalf("expected family revocation and unauthorized, got revoked=%v error=%v", sessions.revoked, err)
	}
}

func TestLoginCreatesHashedSession(t *testing.T) {
	users := &fakeUsers{lookup: domain.User{ID: "user-1", Email: "a@example.com", PasswordHash: "stored-hash"}}
	sessions := &fakeSessions{}
	service := NewService(users, sessions, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, &fakeEvents{})
	pair, err := service.Login(context.Background(), " A@Example.COM ", "correct-password")
	if err != nil {
		t.Fatal(err)
	}
	if pair.AccessToken != "signed-access-token" || pair.ExpiresIn != 900 || pair.RefreshToken == "" {
		t.Fatalf("unexpected token pair: %+v", pair)
	}
	if sessions.token.UserID != "user-1" || sessions.token.FamilyID == "" ||
		sessions.token.ExpiresAt.Before(time.Now().Add(29*24*time.Hour)) ||
		strings.Contains(string(sessions.token.Hash), pair.RefreshToken) {
		t.Fatalf("refresh session was not stored safely: %+v", sessions.token)
	}
}

func TestLoginFailurePaths(t *testing.T) {
	for _, test := range []struct {
		name, email string
		users       *fakeUsers
		hasher      fakeHasher
		tokens      fakeAccessTokens
		sessions    *fakeSessions
		want        error
	}{
		{"invalid email", "not-an-email", &fakeUsers{}, fakeHasher{}, fakeAccessTokens{}, &fakeSessions{}, domain.ErrUnauthorized},
		{"missing user", "a@example.com", &fakeUsers{emailErr: domain.ErrNotFound}, fakeHasher{}, fakeAccessTokens{}, &fakeSessions{}, domain.ErrUnauthorized},
		{"database failure", "a@example.com", &fakeUsers{emailErr: errors.New("database failure")}, fakeHasher{}, fakeAccessTokens{}, &fakeSessions{}, nil},
		{"hash failure", "a@example.com", &fakeUsers{}, fakeHasher{verifyErr: errors.New("hash failure")}, fakeAccessTokens{}, &fakeSessions{}, nil},
		{"signing failure", "a@example.com", &fakeUsers{}, fakeHasher{}, fakeAccessTokens{err: errors.New("signing failure")}, &fakeSessions{}, nil},
		{"session failure", "a@example.com", &fakeUsers{}, fakeHasher{}, fakeAccessTokens{}, &fakeSessions{insertErr: errors.New("session failure")}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(test.users, test.sessions, fakeUnitOfWork{}, test.hasher,
				test.tokens, &fakeEvents{}).Login(context.Background(), test.email, "correct-password")
			if err == nil || (test.want != nil && !errors.Is(err, test.want)) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestRefreshRotatesTokenAndRejectsExpired(t *testing.T) {
	sessions := &fakeSessions{token: domain.RefreshToken{
		ID: "old-token", UserID: "user-1", FamilyID: "family-1", ExpiresAt: time.Now().Add(time.Hour),
	}}
	service := NewService(&fakeUsers{}, sessions, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, &fakeEvents{})
	pair, err := service.Refresh(context.Background(), "old-raw-token")
	if err != nil {
		t.Fatal(err)
	}
	if !sessions.marked || pair.RefreshToken == "" || sessions.token.FamilyID != "family-1" {
		t.Fatalf("refresh did not rotate within family: pair=%+v session=%+v", pair, sessions.token)
	}
	for _, test := range []struct {
		name, raw string
		session   fakeSessions
	}{
		{"empty", "", fakeSessions{}},
		{"too long", strings.Repeat("x", 513), fakeSessions{}},
		{"missing", "missing", fakeSessions{lockErr: domain.ErrNotFound}},
		{"expired", "expired", fakeSessions{token: domain.RefreshToken{ExpiresAt: time.Now().Add(-time.Minute)}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(&fakeUsers{}, &test.session, fakeUnitOfWork{}, fakeHasher{},
				fakeAccessTokens{}, &fakeEvents{}).Refresh(context.Background(), test.raw)
			if !errors.Is(err, domain.ErrUnauthorized) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestRegisterFailurePaths(t *testing.T) {
	for _, test := range []struct {
		name, email, password string
		users                 *fakeUsers
		hasher                fakeHasher
		events                *fakeEvents
	}{
		{"invalid email", "bad", "correct-password", &fakeUsers{}, fakeHasher{}, &fakeEvents{}},
		{"short password", "a@example.com", "short", &fakeUsers{}, fakeHasher{}, &fakeEvents{}},
		{"hash error", "a@example.com", "correct-password", &fakeUsers{}, fakeHasher{hashErr: errors.New("hash error")}, &fakeEvents{}},
		{"create error", "a@example.com", "correct-password", &fakeUsers{createErr: errors.New("create error")}, fakeHasher{}, &fakeEvents{}},
		{"event error", "a@example.com", "correct-password", &fakeUsers{}, fakeHasher{}, &fakeEvents{err: errors.New("event error")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewService(test.users, &fakeSessions{}, fakeUnitOfWork{}, test.hasher,
				fakeAccessTokens{}, test.events).Register(context.Background(), test.email, test.password)
			if err == nil {
				t.Fatal("expected registration failure")
			}
		})
	}
}

func TestUserQueriesAndCursorPage(t *testing.T) {
	now := time.Now()
	users := &fakeUsers{lookup: domain.User{ID: "user-1"}, listed: []domain.User{
		{ID: "user-1", CreatedAt: now}, {ID: "user-2", CreatedAt: now.Add(time.Second)},
	}}
	service := NewService(users, &fakeSessions{}, fakeUnitOfWork{}, fakeHasher{}, fakeAccessTokens{}, &fakeEvents{})
	user, err := service.Me(context.Background(), "user-1")
	if err != nil || user.ID != "user-1" {
		t.Fatalf("me=%+v error=%v", user, err)
	}
	user, err = NewQueries(users).GetUser(context.Background(), "user-1")
	if err != nil || user.ID != "user-1" {
		t.Fatalf("query=%+v error=%v", user, err)
	}
	page, cursor, err := service.ListUsers(context.Background(), nil, 1)
	if err != nil || len(page) != 1 || cursor == nil || cursor.ID != "user-1" || users.listLimit != 2 {
		t.Fatalf("page=%+v cursor=%+v error=%v", page, cursor, err)
	}
	users.listed = users.listed[:1]
	page, cursor, err = service.ListUsers(context.Background(), &Cursor{ID: "user-1", CreatedAt: now}, 10)
	if err != nil || len(page) != 1 || cursor != nil {
		t.Fatalf("last page=%+v cursor=%+v error=%v", page, cursor, err)
	}
	for _, limit := range []int{0, 101} {
		if _, _, err := service.ListUsers(context.Background(), nil, limit); !errors.Is(err, domain.ErrValidation) {
			t.Fatalf("limit %d error=%v", limit, err)
		}
	}
	if _, _, err := service.ListUsers(context.Background(), &Cursor{ID: "user-1"}, 10); !errors.Is(err, domain.ErrValidation) {
		t.Fatalf("invalid cursor error=%v", err)
	}
	users.listErr = errors.New("query failed")
	if _, _, err := service.ListUsers(context.Background(), nil, 10); err == nil {
		t.Fatal("expected list failure")
	}
	users.idErr = domain.ErrNotFound
	if _, err := service.GetUser(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("get error=%v", err)
	}
	if _, err := NewQueries(users).GetUser(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("query error=%v", err)
	}
}

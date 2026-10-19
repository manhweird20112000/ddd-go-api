package httpadapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yourorg/go-template/internal/modules/identity/app"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
)

type fakeService struct {
	registerErr error
}

func (f fakeService) Register(context.Context, string, string) (domain.User, error) {
	if f.registerErr != nil {
		return domain.User{}, f.registerErr
	}
	return domain.User{ID: "user-123", Email: "a@example.com", PasswordHash: "private-hash", CreatedAt: time.Now()}, nil
}
func (fakeService) Login(context.Context, string, string) (app.TokenPair, error) {
	return app.TokenPair{}, nil
}
func (fakeService) Refresh(context.Context, string) (app.TokenPair, error) {
	return app.TokenPair{}, nil
}
func (fakeService) Me(context.Context, string) (domain.User, error) {
	return domain.User{}, nil
}
func (fakeService) ListUsers(context.Context, *app.Cursor, int) ([]domain.User, *app.Cursor, error) {
	return nil, nil, nil
}

func TestRegisterResponseOmitsPasswordHash(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, fakeService{}, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/users/register",
		strings.NewReader(`{"email":"a@example.com","password":"correct-password"}`)))
	if w.Code != http.StatusCreated || !strings.Contains(w.Body.String(), `"email":"a@example.com"`) {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "private-hash") {
		t.Fatal("password hash leaked in HTTP response")
	}
}

func TestRegisterConflictMapsTo409(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoutes(r, fakeService{registerErr: errors.Join(domain.ErrConflict, errors.New("private database detail"))}, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/users/register",
		strings.NewReader(`{"email":"a@example.com","password":"correct-password"}`)))
	if w.Code != http.StatusConflict || strings.Contains(w.Body.String(), "private database detail") {
		t.Fatalf("unexpected response: %d %s", w.Code, w.Body.String())
	}
}

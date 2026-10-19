// Package httpadapter exposes identity use cases over versioned HTTP routes.
package httpadapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/yourorg/go-template/internal/modules/identity/app"
	"github.com/yourorg/go-template/internal/modules/identity/domain"
	"github.com/yourorg/go-template/internal/platform/auth"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Service is the use-case surface consumed by the HTTP adapter.
type Service interface {
	Register(context.Context, string, string) (domain.User, error)
	Login(context.Context, string, string) (app.TokenPair, error)
	Refresh(context.Context, string) (app.TokenPair, error)
	Me(context.Context, string) (domain.User, error)
	ListUsers(context.Context, *app.Cursor, int) ([]domain.User, *app.Cursor, error)
}

type handler struct {
	service  Service
	validate *validator.Validate
}

type credentialsRequest struct {
	Email    string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required,min=12,max=1024"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required,max=512"`
}

type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

type cursorValue struct {
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

// RegisterRoutes mounts identity HTTP routes under /v1.
func RegisterRoutes(router chi.Router, service Service, verifier *auth.Verifier,
	idempotency *httpx.Idempotency, rateLimit func(http.Handler) http.Handler) {
	h := &handler{service: service, validate: validator.New()}
	router.Route("/v1", func(r chi.Router) {
		writes := r
		if rateLimit != nil {
			writes = writes.With(rateLimit)
		}
		if idempotency != nil {
			writes = writes.With(idempotency.Middleware)
		}
		writes.Post("/users/register", h.register)
		writes.Post("/auth/login", h.login)
		writes.Post("/auth/refresh", h.refresh)
		r.With(auth.Require(verifier)).Get("/users/me", h.me)
		r.With(auth.Require(verifier)).Get("/users", h.list)
	})
}

func (h *handler) register(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	user, err := h.service.Register(r.Context(), input.Email, input.Password)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, userResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	var input credentialsRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	pair, err := h.service.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: pair.AccessToken, TokenType: "Bearer", ExpiresIn: pair.ExpiresIn,
		RefreshToken: pair.RefreshToken,
	})
}

func (h *handler) refresh(w http.ResponseWriter, r *http.Request) {
	var input refreshRequest
	if !h.decodeAndValidate(w, r, &input) {
		return
	}
	pair, err := h.service.Refresh(r.Context(), input.RefreshToken)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken: pair.AccessToken, TokenType: "Bearer", ExpiresIn: pair.ExpiresIn,
		RefreshToken: pair.RefreshToken,
	})
}

func (h *handler) me(w http.ResponseWriter, r *http.Request) {
	user, err := h.service.Me(r.Context(), auth.Subject(r.Context()))
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, userResponse{ID: user.ID, Email: user.Email, CreatedAt: user.CreatedAt})
}

func (h *handler) list(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			httpx.WriteError(w, r, http.StatusBadRequest, "validation", "invalid limit")
			return
		}
		limit = parsed
	}
	var after *app.Cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		parsed, err := decodeCursor(raw)
		if err != nil {
			httpx.WriteError(w, r, http.StatusBadRequest, "validation", "invalid cursor")
			return
		}
		after = parsed
	}
	users, next, err := h.service.ListUsers(r.Context(), after, limit)
	if err != nil {
		writeAppError(w, r, err)
		return
	}
	items := make([]map[string]any, 0, len(users))
	for _, user := range users {
		items = append(items, map[string]any{"id": user.ID, "created_at": user.CreatedAt})
	}
	var nextCursor *string
	if next != nil {
		value, err := encodeCursor(next)
		if err != nil {
			writeAppError(w, r, err)
			return
		}
		nextCursor = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "next_cursor": nextCursor})
}

func (h *handler) decodeAndValidate(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return false
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "validation", "invalid JSON body")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			return false
		}
		httpx.WriteError(w, r, http.StatusBadRequest, "validation", "request body must contain one JSON object")
		return false
	}
	if err := h.validate.Struct(value); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, "validation", "invalid request fields")
		return false
	}
	return true
}

func writeAppError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrValidation):
		httpx.WriteError(w, r, http.StatusBadRequest, "validation", "invalid input")
	case errors.Is(err, domain.ErrUnauthorized):
		httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid credentials or token")
	case errors.Is(err, domain.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "user not found")
	case errors.Is(err, domain.ErrConflict):
		httpx.WriteError(w, r, http.StatusConflict, "conflict", "user already exists")
	default:
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal", "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func decodeCursor(raw string) (*app.Cursor, error) {
	if len(raw) > 1024 {
		return nil, errors.New("cursor is too long")
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, err
	}
	var value cursorValue
	if err := json.Unmarshal(decoded, &value); err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(value.ID); err != nil || value.CreatedAt.IsZero() {
		return nil, errors.New("invalid cursor value")
	}
	return &app.Cursor{ID: value.ID, CreatedAt: value.CreatedAt}, nil
}

func encodeCursor(cursor *app.Cursor) (string, error) {
	encoded, err := json.Marshal(cursorValue{ID: cursor.ID, CreatedAt: cursor.CreatedAt})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

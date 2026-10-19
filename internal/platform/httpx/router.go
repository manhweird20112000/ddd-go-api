// Package httpx provides shared HTTP transport behavior.
package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"
)

type requestIDKey struct{}

// RequestID returns the current request identifier.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// NewRouter serves process-level health routes with request IDs.
func NewRouter(readiness func(context.Context) error, logs ...*slog.Logger) chi.Router {
	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	if len(logs) > 0 && logs[0] != nil {
		r.Use(accessLog(logs[0]))
	}
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if err := readiness(req.Context()); err != nil {
			WriteError(w, req, http.StatusServiceUnavailable, "unavailable", "service is not ready")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready"}`))
	})
	return r
}

func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(wrapped, r)
			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			logger.InfoContext(r.Context(), "http_request",
				"method", r.Method, "path", r.URL.Path, "status", status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestID(r.Context()),
				"trace_id", trace.SpanContextFromContext(r.Context()).TraceID().String(),
			)
		})
	}
}

// WriteError writes the public error envelope without exposing internal details.
func WriteError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"code": code, "message": message, "request_id": RequestID(r.Context()),
		},
	})
}

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID(id) {
			var random [16]byte
			if _, err := rand.Read(random[:]); err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			id = hex.EncodeToString(random[:])
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

func validRequestID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_", r) {
			return false
		}
	}
	return true
}

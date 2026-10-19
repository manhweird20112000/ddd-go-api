//go:build integration

package httpx_test

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	pgcontainer "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/yourorg/go-template/internal/modules/identity/migrations"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

func TestIdempotencyReplayConflictAndRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	container, err := pgcontainer.Run(ctx, "postgres:16",
		pgcontainer.BasicWaitStrategies(), pgcontainer.WithDatabase("idempotency_test"),
		pgcontainer.WithUsername("idempotency_test"), pgcontainer.WithPassword(uuid.NewString()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testcontainers.TerminateContainer(container)) })
	url, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	require.NoError(t, migrations.Run(ctx, url, "up"))
	pool, err := pgxpool.New(ctx, url)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	idempotency, err := httpx.NewIdempotency(pool, base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32))))
	require.NoError(t, err)

	var mu sync.Mutex
	calls := 0
	handler := idempotency.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		_, err := db.FromContext(r.Context(), pool).Exec(r.Context(),
			`INSERT INTO identity.outbox (id, event_type, version, aggregate_id, occurred_at, payload)
			 VALUES ($1, 'test.event', 1, $2, now(), '{}')`, uuid.New(), uuid.New())
		if err != nil {
			t.Errorf("insert outbox: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"secret":"token"}`))
	}))
	request := func(key, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/users/register", strings.NewReader(body))
		r.Header.Set("Idempotency-Key", key)
		handler.ServeHTTP(w, r)
		return w
	}

	first := request("key-replay-123", `{"x":1}`)
	require.Equal(t, http.StatusCreated, first.Code)
	second := request("key-replay-123", `{"x":1}`)
	require.Equal(t, first.Body.String(), second.Body.String())
	require.Equal(t, http.StatusConflict, request("key-replay-123", `{"x":2}`).Code)
	require.Equal(t, 1, calls)
	var rowCount int
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox").Scan(&rowCount))
	require.Equal(t, 1, rowCount)
	var simultaneous sync.WaitGroup
	results := make(chan *httptest.ResponseRecorder, 8)
	for range 8 {
		simultaneous.Add(1)
		go func() {
			defer simultaneous.Done()
			results <- request("key-concurrent-123", `{"x":3}`)
		}()
	}
	simultaneous.Wait()
	close(results)
	for result := range results {
		require.Equal(t, http.StatusCreated, result.Code)
		require.Equal(t, first.Body.String(), result.Body.String())
	}
	require.Equal(t, 2, calls)
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox").Scan(&rowCount))
	require.Equal(t, 2, rowCount)
	var ciphertext []byte
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT ciphertext FROM identity.idempotency_keys WHERE key = $1", "key-replay-123").Scan(&ciphertext))
	require.NotContains(t, string(ciphertext), "token")

	failed := idempotency.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := db.FromContext(r.Context(), pool).Exec(r.Context(),
			`INSERT INTO identity.outbox (id, event_type, version, aggregate_id, occurred_at, payload)
			 VALUES ($1, 'test.event', 1, $2, now(), '{}')`, uuid.New(), uuid.New())
		require.NoError(t, err)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/auth/login", strings.NewReader(`{"x":1}`))
	r.Header.Set("Idempotency-Key", "key-rollback-123")
	failed.ServeHTTP(w, r)
	require.Equal(t, http.StatusInternalServerError, w.Code)
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.outbox").Scan(&rowCount))
	require.Equal(t, 2, rowCount)
	require.NoError(t, pool.QueryRow(ctx, "SELECT count(*) FROM identity.idempotency_keys").Scan(&rowCount))
	require.Equal(t, 2, rowCount)
}

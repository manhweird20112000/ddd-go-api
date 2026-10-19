package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthAndReadiness(t *testing.T) {
	ready := false
	h := NewRouter(func(context.Context) error {
		if !ready {
			return errors.New("database unavailable")
		}
		return nil
	})

	for _, tc := range []struct {
		path string
		want int
	}{
		{path: "/healthz", want: http.StatusOK},
		{path: "/readyz", want: http.StatusServiceUnavailable},
	} {
		r := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.path, w.Code, tc.want)
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s: missing request id", tc.path)
		}
	}

	ready = true
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("ready status: got %d", w.Code)
	}
}

func TestReadinessErrorShape(t *testing.T) {
	h := NewRouter(func(context.Context) error { return errors.New("secret db detail") })
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	var body struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != "unavailable" || body.Error.RequestID != w.Header().Get("X-Request-ID") {
		t.Fatalf("unexpected error body: %+v", body)
	}
	if body.Error.Message == "secret db detail" {
		t.Fatal("internal error detail was exposed")
	}
}

func TestRequestIDIsLogged(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	h := NewRouter(func(context.Context) error { return nil }, logger)
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.Header.Set("X-Request-ID", "request-123")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("X-Request-ID") != "request-123" {
		t.Fatal("request id was not propagated")
	}
	if !bytes.Contains(output.Bytes(), []byte(`"request_id":"request-123"`)) {
		t.Fatalf("request id missing from log: %s", output.String())
	}
}

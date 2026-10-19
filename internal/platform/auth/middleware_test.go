package auth

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequireBearerToken(t *testing.T) {
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := NewIssuer(base64.StdEncoding.EncodeToString(private), "test-issuer", "test-api")
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := NewVerifier(issuer.PublicKey(), "test-issuer", "test-api")
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := issuer.Issue(context.Background(), "user-123")
	if err != nil {
		t.Fatal(err)
	}
	handler := Require(verifier)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if Subject(r.Context()) != "user-123" {
			t.Error("subject missing from context")
		}
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range []struct {
		bearer string
		want   int
	}{
		{want: http.StatusUnauthorized},
		{bearer: "Bearer " + token, want: http.StatusOK},
	} {
		r := httptest.NewRequest(http.MethodGet, "/v1/users/me", nil)
		if tc.bearer != "" {
			r.Header.Set("Authorization", tc.bearer)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != tc.want {
			t.Fatalf("got %d, want %d", w.Code, tc.want)
		}
	}
}

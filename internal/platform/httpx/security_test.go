package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityExactOriginAndPreflight(t *testing.T) {
	handler := Security("https://app.example.test")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	request := func(origin, method string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/v1/users", nil)
		r.Header.Set("Origin", origin)
		if method == http.MethodOptions {
			r.Header.Set("Access-Control-Request-Method", http.MethodPost)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	allowed := request("https://app.example.test", http.MethodOptions)
	if allowed.Code != http.StatusNoContent || allowed.Header().Get("Access-Control-Allow-Origin") != "https://app.example.test" {
		t.Fatalf("valid origin preflight: %d %v", allowed.Code, allowed.Header())
	}
	for _, origin := range []string{"https://app.example.test.evil", "https://evil.example.test"} {
		blocked := request(origin, http.MethodPost)
		if blocked.Code != http.StatusForbidden || blocked.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatalf("origin %q accepted: %d", origin, blocked.Code)
		}
	}
	if request("", http.MethodPost).Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header")
	}
}

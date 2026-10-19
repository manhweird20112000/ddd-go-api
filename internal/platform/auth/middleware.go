package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/yourorg/go-template/internal/platform/httpx"
)

type subjectKey struct{}

// Subject returns the verified access-token subject from request context.
func Subject(ctx context.Context) string {
	subject, _ := ctx.Value(subjectKey{}).(string)
	return subject
}

// Require rejects requests without one valid Bearer access token.
func Require(verifier *Verifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			headers := r.Header.Values("Authorization")
			if len(headers) != 1 || len(headers[0]) > 4096 {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid access token")
				return
			}
			scheme, token, ok := strings.Cut(headers[0], " ")
			if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" || strings.ContainsAny(token, " \t") {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid access token")
				return
			}
			subject, err := verifier.Verify(r.Context(), token)
			if err != nil {
				httpx.WriteError(w, r, http.StatusUnauthorized, "unauthorized", "invalid access token")
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), subjectKey{}, subject)))
		})
	}
}

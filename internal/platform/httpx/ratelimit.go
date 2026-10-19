package httpx

import (
	"crypto/sha256"
	"encoding/hex"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var rateScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return count`)

// RateLimit permits at most limit requests per client IP and route each minute.
// It rejects writes when Redis is unavailable so replicas share one limit.
func RateLimit(client *redis.Client, limit int) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if client == nil || limit < 1 {
				WriteError(w, r, http.StatusServiceUnavailable, "unavailable", "rate limiter is unavailable")
				return
			}
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				WriteError(w, r, http.StatusBadRequest, "validation", "invalid client address")
				return
			}
			now := time.Now()
			window := now.Unix() / 60
			fingerprint := sha256.Sum256([]byte(host + "\x00" + r.URL.Path))
			key := "rate:identity:" + hex.EncodeToString(fingerprint[:]) + ":" + strconv.FormatInt(window, 10)
			count, err := rateScript.Run(r.Context(), client, []string{key}, 120).Int64()
			if err != nil {
				WriteError(w, r, http.StatusServiceUnavailable, "unavailable", "rate limiter is unavailable")
				return
			}
			if count > int64(limit) {
				w.Header().Set("Retry-After", strconv.FormatInt(60-now.Unix()%60, 10))
				WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

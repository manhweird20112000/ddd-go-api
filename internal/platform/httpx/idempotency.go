package httpx

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/platform/db"
)

var errHandlerFailed = errors.New("idempotent handler failed")

// Idempotency commits an HTTP write and its encrypted response atomically.
type Idempotency struct {
	pool       *pgxpool.Pool
	tx         *db.TxManager
	requestKey []byte
	aead       cipher.AEAD
}

// NewIdempotency derives request-HMAC and response-encryption keys from env material.
func NewIdempotency(pool *pgxpool.Pool, rawKey string) (*Idempotency, error) {
	if pool == nil {
		return nil, errors.New("idempotency requires a database pool")
	}
	master, err := base64.StdEncoding.DecodeString(rawKey)
	if err != nil || len(master) != 32 {
		return nil, errors.New("IDEMPOTENCY_KEY_BASE64 must encode 32 bytes")
	}
	requestKey := deriveKey(master, "request-hmac-v1")
	block, err := aes.NewCipher(deriveKey(master, "response-aes-v1"))
	if err != nil {
		return nil, fmt.Errorf("initialize response encryption: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize AES-GCM: %w", err)
	}
	return &Idempotency{pool: pool, tx: db.NewTxManager(pool), requestKey: requestKey, aead: aead}, nil
}

// Middleware requires an idempotency key and replays matching encrypted responses.
func (i *Idempotency) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Idempotency-Key")
		if !validIdempotencyKey(key) {
			WriteError(w, r, http.StatusBadRequest, "validation", "valid Idempotency-Key is required")
			return
		}
		limitedBody := http.MaxBytesReader(w, r.Body, 1<<20)
		defer func() { _ = limitedBody.Close() }()
		body, err := io.ReadAll(limitedBody)
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				WriteError(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body is too large")
			} else {
				WriteError(w, r, http.StatusBadRequest, "validation", "cannot read request body")
			}
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		requestHash := i.hashRequest(r, body)
		var response capturedResponse
		var conflict bool
		err = i.tx.WithinTx(r.Context(), func(txCtx context.Context) error {
			querier := db.FromContext(txCtx, i.pool)
			if _, err := querier.Exec(txCtx, `INSERT INTO identity.idempotency_keys
				(scope, key, request_hmac, expires_at) VALUES ($1, $2, $3, $4)
				ON CONFLICT (scope, key) DO NOTHING`, r.URL.Path, key, requestHash, time.Now().Add(24*time.Hour)); err != nil {
				return fmt.Errorf("claim idempotency key: %w", err)
			}
			var storedHash, nonce, ciphertext []byte
			var status sql.NullInt32
			var contentType string
			var expiresAt time.Time
			if err := querier.QueryRow(txCtx, `SELECT request_hmac, status_code, content_type,
				nonce, ciphertext, expires_at FROM identity.idempotency_keys
				WHERE scope = $1 AND key = $2 FOR UPDATE`, r.URL.Path, key).Scan(
				&storedHash, &status, &contentType, &nonce, &ciphertext, &expiresAt,
			); err != nil {
				return fmt.Errorf("lock idempotency key: %w", err)
			}
			if !expiresAt.After(time.Now()) {
				if _, err := querier.Exec(txCtx, `UPDATE identity.idempotency_keys SET
					request_hmac = $3, status_code = NULL, content_type = 'application/json',
					nonce = NULL, ciphertext = NULL, expires_at = $4, created_at = now()
					WHERE scope = $1 AND key = $2`, r.URL.Path, key, requestHash, time.Now().Add(24*time.Hour)); err != nil {
					return fmt.Errorf("reset expired idempotency key: %w", err)
				}
				storedHash = requestHash
				status.Valid = false
			}
			if !hmac.Equal(storedHash, requestHash) {
				conflict = true
				return nil
			}
			if status.Valid {
				plain, err := i.aead.Open(nil, nonce, ciphertext, requestHash)
				if err != nil {
					return fmt.Errorf("decrypt idempotent response: %w", err)
				}
				response = capturedResponse{header: make(http.Header), status: int(status.Int32)}
				response.header.Set("Content-Type", contentType)
				_, _ = response.body.Write(plain)
				return nil
			}
			response = capturedResponse{header: make(http.Header)}
			next.ServeHTTP(&response, r.WithContext(txCtx))
			if response.status == 0 {
				response.status = http.StatusOK
			}
			if response.status >= http.StatusInternalServerError {
				return errHandlerFailed
			}
			nonce = make([]byte, i.aead.NonceSize())
			if _, err := rand.Read(nonce); err != nil {
				return fmt.Errorf("generate response nonce: %w", err)
			}
			// #nosec G407 -- crypto/rand fills the nonce immediately above.
			ciphertext = i.aead.Seal(nil, nonce, response.body.Bytes(), requestHash)
			if _, err := querier.Exec(txCtx, `UPDATE identity.idempotency_keys SET
				status_code = $3, content_type = $4, nonce = $5, ciphertext = $6
				WHERE scope = $1 AND key = $2`, r.URL.Path, key, response.status,
				response.header.Get("Content-Type"), nonce, ciphertext); err != nil {
				return fmt.Errorf("save idempotent response: %w", err)
			}
			return nil
		})
		if errors.Is(err, errHandlerFailed) {
			writeCaptured(w, &response)
			return
		}
		if err != nil {
			WriteError(w, r, http.StatusInternalServerError, "internal", "internal server error")
			return
		}
		if conflict {
			WriteError(w, r, http.StatusConflict, "conflict", "Idempotency-Key was used for another request")
			return
		}
		writeCaptured(w, &response)
	})
}

type capturedResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *capturedResponse) Header() http.Header { return r.header }

func (r *capturedResponse) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
}

func (r *capturedResponse) Write(data []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(data)
}

func writeCaptured(w http.ResponseWriter, response *capturedResponse) {
	w.Header().Set("Cache-Control", "no-store")
	if contentType := response.header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.WriteHeader(response.status)
	_, _ = w.Write(response.body.Bytes())
}

func (i *Idempotency) hashRequest(r *http.Request, body []byte) []byte {
	mac := hmac.New(sha256.New, i.requestKey)
	_, _ = mac.Write([]byte(r.Method))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(r.URL.RequestURI()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write(body)
	return mac.Sum(nil)
}

func deriveKey(master []byte, purpose string) []byte {
	mac := hmac.New(sha256.New, master)
	_, _ = mac.Write([]byte(purpose))
	return mac.Sum(nil)
}

func validIdempotencyKey(key string) bool {
	if len(key) < 8 || len(key) > 128 {
		return false
	}
	for _, r := range key {
		if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_.:", r) {
			return false
		}
	}
	return true
}

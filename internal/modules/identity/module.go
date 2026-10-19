// Package identity wires the identity bounded context without exposing its adapters.
package identity

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/modules/identity/adapter/argon2id"
	httpadapter "github.com/yourorg/go-template/internal/modules/identity/adapter/http"
	"github.com/yourorg/go-template/internal/modules/identity/adapter/inproc"
	"github.com/yourorg/go-template/internal/modules/identity/adapter/postgres"
	identityapp "github.com/yourorg/go-template/internal/modules/identity/app"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/platform/auth"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Module owns the identity HTTP service and its selected adapters.
type Module struct {
	service     *identityapp.Service
	verifier    *auth.Verifier
	idempotency *httpx.Idempotency
}

// New constructs the identity module from a pool and validated process config.
func New(pool *pgxpool.Pool, cfg config.Config) (*Module, error) {
	issuer, err := auth.NewIssuer(cfg.JWTPrivateKey, cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return nil, fmt.Errorf("identity signing key: %w", err)
	}
	verifier, err := auth.NewVerifier(issuer.PublicKey(), cfg.JWTIssuer, cfg.JWTAudience)
	if err != nil {
		return nil, fmt.Errorf("identity verifier: %w", err)
	}
	idempotency, err := httpx.NewIdempotency(pool, cfg.IdempotencyKey)
	if err != nil {
		return nil, fmt.Errorf("identity idempotency key: %w", err)
	}
	repository := postgres.New(pool)
	service := identityapp.NewService(repository, repository, db.NewTxManager(pool), argon2id.New(), issuer, repository)
	return &Module{service: service, verifier: verifier, idempotency: idempotency}, nil
}

// RegisterRoutes mounts the versioned identity API on router.
func (m *Module) RegisterRoutes(router chi.Router, rateLimit func(http.Handler) http.Handler) {
	httpadapter.RegisterRoutes(router, m.service, m.verifier, m.idempotency, rateLimit)
}

// Contract exposes the local identity service to the composition root.
func (m *Module) Contract() contract.Service {
	return inproc.New(m.service)
}

// NewLocalContract exposes identity reads to a worker without signing credentials.
func NewLocalContract(pool *pgxpool.Pool) contract.Service {
	return inproc.New(identityapp.NewQueries(postgres.New(pool)))
}

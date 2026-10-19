// Command newmodule adds a runnable bounded-context skeleton to the template.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const registryPath = "internal/app/registry/registry.go"

func main() {
	name := flag.String("name", "", "lowercase module name, such as billing")
	flag.Parse()
	if err := generate(*name); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(name string) error {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(name) || name == "identity" || name == "notification" {
		return errors.New("name must be a new lowercase module identifier")
	}
	root := filepath.Join("internal", "modules", name)
	if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("module %q already exists or cannot be inspected", name)
	}
	registry, err := os.ReadFile(registryPath)
	if err != nil {
		return fmt.Errorf("read module registry: %w", err)
	}
	contents := string(registry)
	if !strings.Contains(contents, "// newmodule:imports") || !strings.Contains(contents, "// newmodule:entries") {
		return errors.New("module registry lacks generation markers")
	}
	if strings.Contains(contents, `Name: "`+name+`"`) {
		return fmt.Errorf("module %q is already registered", name)
	}
	for path, source := range files {
		path = filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("create module directories: %w", err)
		}
		source = strings.ReplaceAll(source, "__NAME__", name)
		source = strings.ReplaceAll(source, "__TITLE__", strings.ToUpper(name[:1])+name[1:])
		if filepath.Ext(path) == ".go" {
			formatted, err := format.Source([]byte(source))
			if err != nil {
				return fmt.Errorf("format %s: %w", path, err)
			}
			source = string(formatted)
		}
		if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	imports := `	__NAME__ "github.com/yourorg/go-template/internal/modules/__NAME__"
	__NAME__migrations "github.com/yourorg/go-template/internal/modules/__NAME__/migrations"
	// newmodule:imports`
	entry := `	{Name: "__NAME__", Migrate: __NAME__migrations.Run,
		New: func(pool *pgxpool.Pool) HTTPModule { return __NAME__.New(pool) }},
	// newmodule:entries`
	contents = strings.Replace(contents, "\t// newmodule:imports", strings.ReplaceAll(imports, "__NAME__", name), 1)
	contents = strings.Replace(contents, "\t// newmodule:entries", strings.ReplaceAll(entry, "__NAME__", name), 1)
	formatted, err := format.Source([]byte(contents))
	if err != nil {
		return fmt.Errorf("format module registry: %w", err)
	}
	// #nosec G703 -- registryPath is a fixed repository-relative path.
	if err := os.WriteFile(registryPath, formatted, 0o600); err != nil {
		return fmt.Errorf("register module: %w", err)
	}
	fmt.Printf("generated and registered %s; run make sqlc and provision %s_DATABASE_URL for deployment\n", name, strings.ToUpper(name))
	return nil
}

var files = map[string]string{
	"domain/status.go": `// Package domain contains the __NAME__ business model.
package domain

// Status describes the module's operational state.
type Status struct {
	Name string ` + "`json:\"name\"`" + `
	Ready bool ` + "`json:\"ready\"`" + `
}
`,
	"contract/status.go": `// Package contract exposes stable __NAME__ values to other modules.
package contract

import "context"

// Status is the module's public operational view.
type Status struct {
	Name string ` + "`json:\"name\"`" + `
	Ready bool ` + "`json:\"ready\"`" + `
}

// Service is the cross-module __NAME__ contract.
type Service interface { Status(context.Context) (Status, error) }
`,
	"app/service.go": `// Package app implements __NAME__ use cases and owns its ports.
package app

import (
	"context"
	"github.com/yourorg/go-template/internal/modules/__NAME__/contract"
	"github.com/yourorg/go-template/internal/modules/__NAME__/domain"
)

// Repository is the application-owned persistence port.
type Repository interface { Ready(context.Context) error }

// Service provides the __NAME__ status use case.
type Service struct { repository Repository }

// NewService constructs a status service.
func NewService(repository Repository) *Service { return &Service{repository: repository} }

// Status checks the module's own database schema.
func (s *Service) Status(ctx context.Context) (contract.Status, error) {
	if err := s.repository.Ready(ctx); err != nil { return contract.Status{}, err }
	state := domain.Status{Name: "__NAME__", Ready: true}
	return contract.Status{Name: state.Name, Ready: state.Ready}, nil
}
`,
	"app/service_test.go": `package app

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct { err error }
func (f fakeRepository) Ready(context.Context) error { return f.err }

func TestStatus(t *testing.T) {
	for _, test := range []struct{name string; err error; ready bool}{
		{"available", nil, true}, {"unavailable", errors.New("database down"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, err := NewService(fakeRepository{err: test.err}).Status(context.Background())
			if (err == nil) != test.ready || status.Ready != test.ready {
				t.Fatalf("status=%+v error=%v", status, err)
			}
		})
	}
}
`,
	"adapter/postgres/repository.go": `// Package postgres persists __NAME__ data in its own schema.
package postgres

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository implements the __NAME__ application persistence port.
type Repository struct { pool *pgxpool.Pool }

// New constructs a PostgreSQL repository.
func New(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// Ready checks that the module's migration has run.
func (r *Repository) Ready(ctx context.Context) error {
	var present bool
	if err := r.pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM __NAME__.module_state)").Scan(&present); err != nil {
		return fmt.Errorf("read __NAME__ state: %w", err)
	}
	if !present { return fmt.Errorf("__NAME__ state is empty") }
	return nil
}
`,
	"adapter/http/handler.go": `// Package httpadapter exposes __NAME__ over HTTP.
package httpadapter

import (
	"context"
	"encoding/json"
	"net/http"
	"github.com/go-chi/chi/v5"
	"github.com/yourorg/go-template/internal/modules/__NAME__/contract"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Service is the HTTP adapter's use-case port.
type Service interface { Status(context.Context) (contract.Status, error) }

// RegisterRoutes exposes the read-only module status endpoint.
func RegisterRoutes(router chi.Router, service Service) {
	router.Get("/v1/__NAME__/status", func(w http.ResponseWriter, r *http.Request) {
		status, err := service.Status(r.Context())
		if err != nil {
			httpx.WriteError(w, r, http.StatusServiceUnavailable, "unavailable", "module is unavailable")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
}
`,
	"module.go": `// Package __NAME__ wires the __NAME__ bounded context.
package __NAME__

import (
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	httpadapter "github.com/yourorg/go-template/internal/modules/__NAME__/adapter/http"
	"github.com/yourorg/go-template/internal/modules/__NAME__/adapter/postgres"
	"github.com/yourorg/go-template/internal/modules/__NAME__/app"
)

// Module owns the __NAME__ adapters and application service.
type Module struct { service *app.Service }

// New constructs the module from its PostgreSQL pool.
func New(pool *pgxpool.Pool) *Module { return &Module{service: app.NewService(postgres.New(pool))} }

// RegisterRoutes mounts the module's HTTP endpoints.
func (m *Module) RegisterRoutes(router chi.Router) { httpadapter.RegisterRoutes(router, m.service) }
`,
	"migrations/fs.go": `// Package migrations embeds __NAME__ schema changes.
package migrations

import "embed"

// Files contains SQL migration files.
//go:embed *.sql
var Files embed.FS
`,
	"migrations/run.go": `package migrations

import (
	"context"
	"github.com/yourorg/go-template/internal/platform/db"
)

// Run applies or rolls back __NAME__ migrations.
func Run(ctx context.Context, databaseURL, direction string) error {
	return db.RunMigrations(ctx, databaseURL, "__NAME__", direction, Files)
}
`,
	"migrations/000001_module.up.sql": `CREATE SCHEMA IF NOT EXISTS __NAME__;
CREATE TABLE __NAME__.module_state (id integer PRIMARY KEY, created_at timestamptz NOT NULL DEFAULT now());
INSERT INTO __NAME__.module_state (id) VALUES (1);
`,
	"migrations/000001_module.down.sql": `DROP TABLE __NAME__.module_state;
`,
	"queries.sql": `-- name: ModuleReady :one
SELECT EXISTS(SELECT 1 FROM __NAME__.module_state);
`,
	"sqlc.yaml": `version: "2"
sql:
  - engine: postgresql
    schema: migrations/000001_module.up.sql
    queries: queries.sql
    gen:
      go:
        package: sqlcgen
        out: adapter/postgres/sqlcgen
        sql_package: pgx/v5
        emit_interface: true
`,
}

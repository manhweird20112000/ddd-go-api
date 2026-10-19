// Package notification wires the notification bounded context.
package notification

import (
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yourorg/go-template/internal/modules/notification/adapter/inproc"
	"github.com/yourorg/go-template/internal/modules/notification/adapter/logsender"
	"github.com/yourorg/go-template/internal/modules/notification/adapter/postgres"
	"github.com/yourorg/go-template/internal/modules/notification/app"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/eventbus"
)

// Module owns notification's selected adapters.
type Module struct{ service *app.Service }

// New constructs notification over its database pool.
func New(pool *pgxpool.Pool, logger *slog.Logger) *Module {
	return &Module{service: app.NewService(postgres.New(pool), db.NewTxManager(pool), logsender.New(logger))}
}

// Subscribe registers the module's in-process event consumer.
func (m *Module) Subscribe(bus *eventbus.InProcess) error {
	return inproc.Subscribe(bus, m.service)
}

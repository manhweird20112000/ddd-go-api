// Package registry lists module migration steps and optional HTTP modules.
package registry

import (
	"context"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	identitymigrations "github.com/yourorg/go-template/internal/modules/identity/migrations"
	notificationmigrations "github.com/yourorg/go-template/internal/modules/notification/migrations"
	// newmodule:imports
)

// HTTPModule registers its own versioned routes.
type HTTPModule interface{ RegisterRoutes(chi.Router) }

// Entry identifies a module's migration and optional HTTP construction.
type Entry struct {
	Name    string
	Migrate func(context.Context, string, string) error
	New     func(*pgxpool.Pool) HTTPModule
}

var entries = []Entry{
	{Name: "identity", Migrate: identitymigrations.Run},
	{Name: "notification", Migrate: notificationmigrations.Run},
	// newmodule:entries
}

// All returns a copy of registered module entries in migration order.
func All() []Entry { return append([]Entry(nil), entries...) }

// Lookup returns a registered module by name.
func Lookup(name string) (Entry, bool) {
	for _, entry := range entries {
		if entry.Name == name {
			return entry, true
		}
	}
	return Entry{}, false
}

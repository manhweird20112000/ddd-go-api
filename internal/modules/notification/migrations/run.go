package migrations

import (
	"context"

	"github.com/yourorg/go-template/internal/platform/db"
)

// Run applies notification migrations in its isolated schema.
func Run(ctx context.Context, databaseURL, direction string) error {
	return db.RunMigrations(ctx, databaseURL, "notification", direction, Files)
}

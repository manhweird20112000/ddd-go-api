package migrations

import (
	"context"

	"github.com/yourorg/go-template/internal/platform/db"
)

// Run applies all pending identity migrations or rolls back one migration.
func Run(ctx context.Context, databaseURL, direction string) error {
	return db.RunMigrations(ctx, databaseURL, "identity", direction, Files)
}

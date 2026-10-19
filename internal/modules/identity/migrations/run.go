package migrations

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// Run applies all pending identity migrations or rolls back one migration.
func Run(ctx context.Context, databaseURL, direction string) (err error) {
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return errors.New("open migration database failed")
	}
	defer func() {
		if closeErr := database.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close migration database: %w", closeErr))
		}
	}()
	database.SetMaxOpenConns(2)
	if _, err := database.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS identity"); err != nil {
		return fmt.Errorf("create identity schema: %w", err)
	}
	source, err := iofs.New(Files, ".")
	if err != nil {
		return fmt.Errorf("open identity migrations: %w", err)
	}
	driver, err := migratepgx.WithInstance(database, &migratepgx.Config{
		SchemaName: "identity", MigrationsTable: "schema_migrations",
		StatementTimeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create identity migration driver: %w", err)
	}
	migrator, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create identity migrator: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := migrator.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()
	switch direction {
	case "up":
		err = migrator.Up()
	case "down":
		err = migrator.Steps(-1)
	default:
		return fmt.Errorf("unsupported migration direction %q", direction)
	}
	if errors.Is(err, migrate.ErrNoChange) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("migrate identity %s: %w", direction, err)
	}
	return nil
}

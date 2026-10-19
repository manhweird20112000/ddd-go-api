package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"time"

	"github.com/golang-migrate/migrate/v4"
	migratepgx "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"
)

// RunMigrations applies a module's embedded migrations under its own schema.
func RunMigrations(ctx context.Context, databaseURL, schema, direction string, files fs.FS) (err error) {
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(schema) {
		return fmt.Errorf("unsupported migration schema %q", schema)
	}
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
	if _, err := database.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+schema); err != nil {
		return fmt.Errorf("create %s schema: %w", schema, err)
	}
	source, err := iofs.New(files, ".")
	if err != nil {
		return fmt.Errorf("open %s migrations: %w", schema, err)
	}
	driver, err := migratepgx.WithInstance(database, &migratepgx.Config{
		SchemaName: schema, MigrationsTable: "schema_migrations",
		StatementTimeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create %s migration driver: %w", schema, err)
	}
	migrator, err := migrate.NewWithInstance("iofs", source, "pgx5", driver)
	if err != nil {
		return fmt.Errorf("create %s migrator: %w", schema, err)
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
		return fmt.Errorf("migrate %s %s: %w", schema, direction, err)
	}
	return nil
}

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	identitymigrations "github.com/yourorg/go-template/internal/modules/identity/migrations"
	notificationmigrations "github.com/yourorg/go-template/internal/modules/notification/migrations"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/logging"
)

func main() {
	logger := logging.New(os.Stdout)
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down") {
		logger.Error("usage: migrate up|down")
		os.Exit(2)
	}
	cfg, err := config.Parse(nil)
	if err != nil {
		logger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	steps := []struct {
		name string
		run  func(context.Context, string, string) error
	}{
		{"identity", identitymigrations.Run},
		{"notification", notificationmigrations.Run},
	}
	if os.Args[1] == "down" {
		steps[0], steps[1] = steps[1], steps[0]
	}
	for _, step := range steps {
		if err := step.run(ctx, cfg.DatabaseURL, os.Args[1]); err != nil {
			logger.Error("migration failed", "module", step.name, slog.Any("error", err))
			os.Exit(1)
		}
	}
	logger.Info("migrations complete", "direction", os.Args[1])
}

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourorg/go-template/internal/modules/identity/migrations"
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
	if err := migrations.Run(ctx, cfg.DatabaseURL, os.Args[1]); err != nil {
		logger.Error("identity migration failed", slog.Any("error", err))
		os.Exit(1)
	}
	logger.Info("identity migration complete", "direction", os.Args[1])
}

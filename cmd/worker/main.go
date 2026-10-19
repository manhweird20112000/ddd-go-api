package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourorg/go-template/internal/app"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/logging"
)

func main() {
	logger := logging.New(os.Stdout)
	cfg, err := config.Parse(nil)
	if err != nil {
		logger.Error("invalid configuration", slog.Any("error", err))
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.RunWorker(ctx, cfg, logger); err != nil {
		logger.Error("worker stopped", slog.Any("error", err))
		os.Exit(1)
	}
}

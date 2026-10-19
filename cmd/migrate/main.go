package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/yourorg/go-template/internal/app/registry"
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
	steps := registry.All()
	if os.Args[1] == "down" {
		for left, right := 0, len(steps)-1; left < right; left, right = left+1, right-1 {
			steps[left], steps[right] = steps[right], steps[left]
		}
	}
	for _, step := range steps {
		if err := step.Migrate(ctx, cfg.DatabaseURL, os.Args[1]); err != nil {
			logger.Error("migration failed", "module", step.Name, slog.Any("error", err))
			os.Exit(1)
		}
	}
	logger.Info("migrations complete", "direction", os.Args[1])
}

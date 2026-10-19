package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/yourorg/go-template/internal/modules/notification"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/eventbus"
	"github.com/yourorg/go-template/internal/platform/outbox"
)

// RunWorker relays committed events until its context is canceled.
func RunWorker(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := db.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	bus := eventbus.NewInProcess()
	if err := notification.New(pool, logger).Subscribe(bus); err != nil {
		return fmt.Errorf("subscribe notification: %w", err)
	}
	relay, err := outbox.NewRelay(pool, "identity", bus, logger)
	if err != nil {
		return err
	}
	logger.InfoContext(ctx, "worker_started")
	return relay.Run(ctx)
}

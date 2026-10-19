package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/yourorg/go-template/internal/modules/identity"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
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
	var localBus *eventbus.InProcess
	var broker *eventbus.NATS
	var publisher eventbus.Publisher
	if cfg.EventBus == "nats" {
		broker, err = eventbus.NewNATS(startupCtx, cfg.NATSURL, logger)
		if err != nil {
			return err
		}
		defer broker.Close()
		publisher = broker
	} else {
		localBus = eventbus.NewInProcess()
		publisher = localBus
	}
	var notificationModule *notification.Module
	if cfg.HasModule("notification") {
		var localContract contract.Service
		if cfg.HasModule("identity") {
			localContract = identity.NewLocalContract(pool)
		}
		identityService, closer, err := resolveIdentity(startupCtx, cfg, localContract)
		if err != nil {
			return err
		}
		if closer != nil {
			defer func() { _ = closer.Close() }()
		}
		notificationModule = notification.New(pool, logger, identityService)
		if localBus != nil {
			if err := notificationModule.Subscribe(localBus); err != nil {
				return fmt.Errorf("subscribe notification: %w", err)
			}
		}
	}
	group, runCtx := errgroup.WithContext(ctx)
	if cfg.HasModule("identity") {
		relay, err := outbox.NewRelay(pool, "identity", publisher, logger)
		if err != nil {
			return err
		}
		group.Go(func() error { return relay.Run(runCtx) })
	}
	if broker != nil && notificationModule != nil {
		group.Go(func() error { return broker.RunConsumer(runCtx, notificationModule.Handler()) })
	}
	logger.InfoContext(ctx, "worker_started", "modules", cfg.EnabledModules, "event_bus", cfg.EventBus)
	return group.Wait()
}

package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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
	var identityPool, notificationPool *pgxpool.Pool
	var err error
	if cfg.HasModule("identity") {
		identityPool, err = db.Open(startupCtx, cfg.ModuleDatabaseURL("identity"))
		if err != nil {
			return fmt.Errorf("open identity database: %w", err)
		}
		defer identityPool.Close()
	}
	if cfg.HasModule("notification") {
		notificationPool, err = db.Open(startupCtx, cfg.ModuleDatabaseURL("notification"))
		if err != nil {
			return fmt.Errorf("open notification database: %w", err)
		}
		defer notificationPool.Close()
	}
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
			localContract = identity.NewLocalContract(identityPool)
		}
		identityService, closer, err := resolveIdentity(startupCtx, cfg, localContract)
		if err != nil {
			return err
		}
		if closer != nil {
			defer func() { _ = closer.Close() }()
		}
		notificationModule = notification.New(notificationPool, logger, identityService)
		if localBus != nil {
			if err := notificationModule.Subscribe(localBus); err != nil {
				return fmt.Errorf("subscribe notification: %w", err)
			}
		}
	}
	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return db.RunCleanup(runCtx, identityPool, notificationPool, logger) })
	var relay *outbox.Relay
	if cfg.HasModule("identity") {
		relay, err = outbox.NewRelay(identityPool, "identity", publisher, logger)
		if err != nil {
			return err
		}
		group.Go(func() error { return relay.Run(runCtx) })
	}
	if broker != nil && notificationModule != nil {
		group.Go(func() error { return broker.RunConsumer(runCtx, notificationModule.Handler()) })
	}
	logger.InfoContext(ctx, "worker_started", "modules", cfg.EnabledModules, "event_bus", cfg.EventBus)
	waitErr := group.Wait()
	if relay != nil {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer flushCancel()
		if _, err := relay.RunOnce(flushCtx, 50); err != nil {
			waitErr = errors.Join(waitErr, fmt.Errorf("flush identity outbox: %w", err))
		}
	}
	return waitErr
}

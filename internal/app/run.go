// Package app is the composition root for process entrypoints.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/yourorg/go-template/internal/app/registry"
	"github.com/yourorg/go-template/internal/modules/identity"
	grpcadapter "github.com/yourorg/go-template/internal/modules/identity/adapter/grpc"
	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/notification"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/eventbus"
	identityv1 "github.com/yourorg/go-template/internal/platform/grpcx/gen/identity/v1"
	"github.com/yourorg/go-template/internal/platform/httpx"
	"github.com/yourorg/go-template/internal/platform/metrics"
	"github.com/yourorg/go-template/internal/platform/outbox"
	"github.com/yourorg/go-template/internal/platform/tracing"
)

// Run serves HTTP and metrics until ctx is canceled or a server fails.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
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
	extraPools := make(map[string]*pgxpool.Pool)
	extraModules := make(map[string]registry.HTTPModule)
	for _, name := range cfg.ModuleNames() {
		entry, found := registry.Lookup(name)
		if !found {
			return fmt.Errorf("unregistered module %q", name)
		}
		if entry.New == nil {
			continue
		}
		modulePool, err := db.Open(startupCtx, cfg.ModuleDatabaseURL(name))
		if err != nil {
			return fmt.Errorf("open %s database: %w", name, err)
		}
		defer modulePool.Close()
		extraPools[name] = modulePool
		extraModules[name] = entry.New(modulePool)
	}
	var identityModule *identity.Module
	if cfg.HasModule("identity") {
		identityModule, err = identity.New(identityPool, cfg)
		if err != nil {
			return err
		}
	}

	var cache *redis.Client
	if cfg.RedisURL != "" {
		options, err := redis.ParseURL(cfg.RedisURL)
		if err != nil {
			return fmt.Errorf("parse Redis URL: %w", err)
		}
		cache = redis.NewClient(options)
		defer func() {
			if err := cache.Close(); err != nil {
				logger.Error("close Redis", "error", err)
			}
		}()
		if err := cache.Ping(startupCtx).Err(); err != nil {
			return fmt.Errorf("ping Redis: %w", err)
		}
	}
	if identityModule != nil && cache == nil {
		return errors.New("identity API requires REDIS_URL for rate limiting")
	}

	traceProvider, err := tracing.New(startupCtx, cfg.OTLPEndpoint, cfg.ServiceName, cfg.OTLPInsecure)
	if err != nil {
		return err
	}
	defer shutdownTelemetry(logger, cfg.ShutdownTimeout, traceProvider.Shutdown)
	meterProvider, metricsHandler, err := metrics.New(startupCtx, cfg.OTLPEndpoint, cfg.ServiceName, cfg.OTLPInsecure)
	if err != nil {
		return err
	}
	defer shutdownTelemetry(logger, cfg.ShutdownTimeout, meterProvider.Shutdown)
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
		if identityModule != nil {
			localContract = identityModule.Contract()
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
	var relay *outbox.Relay
	if identityModule != nil {
		relay, err = outbox.NewRelay(identityPool, "identity", publisher, logger)
		if err != nil {
			return err
		}
	}

	readiness := func(ctx context.Context) error {
		checkCtx, checkCancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
		defer checkCancel()
		for name, pool := range map[string]*pgxpool.Pool{"identity": identityPool, "notification": notificationPool} {
			if pool != nil {
				if err := pool.Ping(checkCtx); err != nil {
					return fmt.Errorf("%s database readiness: %w", name, err)
				}
			}
		}
		for name, pool := range extraPools {
			if err := pool.Ping(checkCtx); err != nil {
				return fmt.Errorf("%s database readiness: %w", name, err)
			}
		}
		if cache != nil {
			if err := cache.Ping(checkCtx).Err(); err != nil {
				return fmt.Errorf("redis readiness: %w", err)
			}
		}
		if broker != nil {
			if err := broker.Ready(checkCtx); err != nil {
				return fmt.Errorf("NATS readiness: %w", err)
			}
		}
		return nil
	}
	propagators := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	router := httpx.NewRouter(readiness, logger)
	if identityModule != nil {
		identityModule.RegisterRoutes(router, httpx.RateLimit(cache, 20))
	}
	for _, name := range cfg.ModuleNames() {
		if module := extraModules[name]; module != nil {
			module.RegisterRoutes(router)
		}
	}
	handler := otelhttp.NewHandler(httpx.Security(cfg.CORSAllowedOrigins)(router), "http.server",
		otelhttp.WithTracerProvider(traceProvider),
		otelhttp.WithMeterProvider(meterProvider),
		otelhttp.WithPropagators(propagators))
	httpServer := &http.Server{
		Addr: cfg.HTTPAddr, Handler: handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	metricsMux := http.NewServeMux()
	metricsMux.Handle("/metrics", metricsHandler)
	metricsServer := &http.Server{
		Addr: cfg.MetricsAddr, Handler: metricsMux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	var grpcServer *grpc.Server
	var grpcListener net.Listener
	if identityModule != nil {
		grpcListener, err = net.Listen("tcp", cfg.GRPCAddr)
		if err != nil {
			return fmt.Errorf("listen for identity gRPC: %w", err)
		}
		grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler(
			otelgrpc.WithTracerProvider(traceProvider), otelgrpc.WithPropagators(propagators))))
		identityv1.RegisterIdentityServiceServer(grpcServer, grpcadapter.New(identityModule.Contract()))
	}

	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return db.RunCleanup(runCtx, identityPool, notificationPool, logger) })
	if relay != nil {
		group.Go(func() error { return relay.Run(runCtx) })
	}
	if broker != nil && notificationModule != nil {
		group.Go(func() error { return broker.RunConsumer(runCtx, notificationModule.Handler()) })
	}
	if grpcServer != nil {
		group.Go(func() error {
			if err := grpcServer.Serve(grpcListener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
				return fmt.Errorf("serve identity gRPC: %w", err)
			}
			return nil
		})
	}
	group.Go(func() error { return serve(httpServer) })
	group.Go(func() error { return serve(metricsServer) })
	group.Go(func() error {
		<-runCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		httpErr := errors.Join(httpServer.Shutdown(shutdownCtx), metricsServer.Shutdown(shutdownCtx))
		if grpcServer != nil {
			stopped := make(chan struct{})
			go func() {
				grpcServer.GracefulStop()
				close(stopped)
			}()
			select {
			case <-stopped:
			case <-shutdownCtx.Done():
				grpcServer.Stop()
				<-stopped
			}
		}
		return httpErr
	})
	logger.InfoContext(ctx, "api_started", "http_addr", cfg.HTTPAddr,
		"grpc_addr", cfg.GRPCAddr, "metrics_addr", cfg.MetricsAddr, "modules", cfg.EnabledModules)
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

func serve(server *http.Server) error {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve %s: %w", server.Addr, err)
	}
	return nil
}

func shutdownTelemetry(logger *slog.Logger, timeout time.Duration, shutdown func(context.Context) error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		logger.Error("shutdown telemetry", "error", err)
	}
}

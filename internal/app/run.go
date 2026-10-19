// Package app is the composition root for process entrypoints.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/propagation"
	"golang.org/x/sync/errgroup"

	"github.com/yourorg/go-template/internal/modules/identity"
	"github.com/yourorg/go-template/internal/platform/config"
	"github.com/yourorg/go-template/internal/platform/db"
	"github.com/yourorg/go-template/internal/platform/httpx"
	"github.com/yourorg/go-template/internal/platform/metrics"
	"github.com/yourorg/go-template/internal/platform/tracing"
)

// Run serves HTTP and metrics until ctx is canceled or a server fails.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	startupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := db.Open(startupCtx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	identityModule, err := identity.New(pool, cfg)
	if err != nil {
		return err
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

	readiness := func(ctx context.Context) error {
		checkCtx, checkCancel := context.WithTimeout(ctx, cfg.ReadyTimeout)
		defer checkCancel()
		if err := pool.Ping(checkCtx); err != nil {
			return fmt.Errorf("database readiness: %w", err)
		}
		if cache != nil {
			if err := cache.Ping(checkCtx).Err(); err != nil {
				return fmt.Errorf("redis readiness: %w", err)
			}
		}
		return nil
	}
	propagators := propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})
	router := httpx.NewRouter(readiness, logger)
	identityModule.RegisterRoutes(router)
	handler := otelhttp.NewHandler(router, "http.server",
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

	group, runCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return serve(httpServer) })
	group.Go(func() error { return serve(metricsServer) })
	group.Go(func() error {
		<-runCtx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer shutdownCancel()
		return errors.Join(httpServer.Shutdown(shutdownCtx), metricsServer.Shutdown(shutdownCtx))
	})
	logger.InfoContext(ctx, "api_started", "http_addr", cfg.HTTPAddr, "metrics_addr", cfg.MetricsAddr)
	return group.Wait()
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

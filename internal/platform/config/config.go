// Package config loads and validates process configuration from environment variables.
package config

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config contains process-level configuration. Never log the complete value.
type Config struct {
	HTTPAddr         string        `env:"HTTP_ADDR" envDefault:":8080"`
	GRPCAddr         string        `env:"GRPC_ADDR" envDefault:":9090"`
	MetricsAddr      string        `env:"METRICS_ADDR" envDefault:":9100"`
	DatabaseURL      string        `env:"DATABASE_URL,required"`
	RedisURL         string        `env:"REDIS_URL"`
	OTLPEndpoint     string        `env:"OTLP_ENDPOINT"`
	OTLPInsecure     bool          `env:"OTLP_INSECURE" envDefault:"false"`
	ServiceName      string        `env:"SERVICE_NAME" envDefault:"go-template"`
	JWTPrivateKey    string        `env:"JWT_PRIVATE_KEY_BASE64"`
	JWTIssuer        string        `env:"JWT_ISSUER" envDefault:"go-template"`
	JWTAudience      string        `env:"JWT_AUDIENCE" envDefault:"go-template-api"`
	EnabledModules   string        `env:"ENABLED_MODULES" envDefault:"identity,notification"`
	EventBus         string        `env:"EVENT_BUS" envDefault:"inproc"`
	NATSURL          string        `env:"NATS_URL"`
	IdentityGRPCAddr string        `env:"IDENTITY_GRPC_ADDR"`
	ShutdownTimeout  time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"15s"`
	ReadyTimeout     time.Duration `env:"READY_TIMEOUT" envDefault:"2s"`
}

// Parse reads a supplied environment map and rejects invalid startup settings.
func Parse(environment map[string]string) (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: environment}); err != nil {
		return Config{}, fmt.Errorf("parse environment: %w", err)
	}
	for name, addr := range map[string]string{
		"HTTP_ADDR": cfg.HTTPAddr, "GRPC_ADDR": cfg.GRPCAddr, "METRICS_ADDR": cfg.MetricsAddr,
	} {
		if err := validateAddress(name, addr); err != nil {
			return Config{}, err
		}
	}
	if cfg.HTTPAddr == cfg.MetricsAddr || cfg.HTTPAddr == cfg.GRPCAddr || cfg.GRPCAddr == cfg.MetricsAddr {
		return Config{}, fmt.Errorf("HTTP_ADDR, GRPC_ADDR, and METRICS_ADDR must differ")
	}
	if err := validateURL("DATABASE_URL", cfg.DatabaseURL, true, "postgres", "postgresql"); err != nil {
		return Config{}, err
	}
	if cfg.RedisURL != "" {
		if err := validateURL("REDIS_URL", cfg.RedisURL, false, "redis", "rediss"); err != nil {
			return Config{}, err
		}
	}
	if cfg.ShutdownTimeout <= 0 || cfg.ReadyTimeout <= 0 {
		return Config{}, fmt.Errorf("shutdown and readiness timeouts must be positive")
	}
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return Config{}, fmt.Errorf("SERVICE_NAME must not be empty")
	}
	if cfg.OTLPEndpoint != "" {
		if err := validateAddress("OTLP_ENDPOINT", cfg.OTLPEndpoint); err != nil {
			return Config{}, err
		}
	}
	modules := make(map[string]bool)
	for _, name := range strings.Split(cfg.EnabledModules, ",") {
		if (name != "identity" && name != "notification") || modules[name] {
			return Config{}, fmt.Errorf("ENABLED_MODULES must list identity and/or notification once")
		}
		modules[name] = true
	}
	if cfg.EventBus != "inproc" && cfg.EventBus != "nats" {
		return Config{}, fmt.Errorf("EVENT_BUS must be inproc or nats")
	}
	if cfg.EventBus == "nats" {
		if err := validateURL("NATS_URL", cfg.NATSURL, false, "nats", "tls"); err != nil {
			return Config{}, err
		}
	}
	if cfg.HasModule("notification") && !cfg.HasModule("identity") {
		if err := validateAddress("IDENTITY_GRPC_ADDR", cfg.IdentityGRPCAddr); err != nil {
			return Config{}, err
		}
	}
	if cfg.EventBus == "inproc" && len(modules) != 2 {
		return Config{}, fmt.Errorf("inproc event bus requires both modules")
	}
	return cfg, nil
}

// HasModule reports whether this process hosts the named module.
func (c Config) HasModule(name string) bool {
	for _, enabled := range strings.Split(c.EnabledModules, ",") {
		if enabled == name {
			return true
		}
	}
	return false
}

func validateAddress(name, addr string) error {
	_, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%s must be a host:port address", name)
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s must have a valid TCP port", name)
	}
	return nil
}

func validateURL(name, raw string, credentialsRequired bool, schemes ...string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (credentialsRequired && u.User == nil) {
		return fmt.Errorf("%s must be a valid URL with host", name)
	}
	for _, scheme := range schemes {
		if u.Scheme == scheme {
			return nil
		}
	}
	return fmt.Errorf("%s uses an unsupported scheme", name)
}

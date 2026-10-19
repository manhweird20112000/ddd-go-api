package config

import (
	"strings"
	"testing"
)

func TestParseRequiresDatabaseURL(t *testing.T) {
	_, err := Parse(map[string]string{})
	if err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got %v", err)
	}
}

func TestParseDefaultsAndValidatesPorts(t *testing.T) {
	cfg, err := Parse(map[string]string{
		"DATABASE_URL": "postgres://app@localhost:5432/app?sslmode=disable",
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" || cfg.MetricsAddr != ":9100" {
		t.Fatalf("unexpected addresses: %q, %q", cfg.HTTPAddr, cfg.MetricsAddr)
	}

	_, err = Parse(map[string]string{
		"DATABASE_URL": "postgres://app@localhost:5432/app?sslmode=disable",
		"HTTP_ADDR":    "not-an-address",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP_ADDR") {
		t.Fatalf("expected HTTP_ADDR error, got %v", err)
	}
}

func TestParseAllowsUnauthenticatedLocalRedis(t *testing.T) {
	_, err := Parse(map[string]string{
		"DATABASE_URL": "postgres://app@localhost:5432/app?sslmode=disable",
		"REDIS_URL":    "redis://localhost:6379/0",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestParseRejectsNonnumericPort(t *testing.T) {
	_, err := Parse(map[string]string{
		"DATABASE_URL": "postgres://app@localhost:5432/app?sslmode=disable",
		"HTTP_ADDR":    ":invalid",
	})
	if err == nil || !strings.Contains(err.Error(), "HTTP_ADDR") {
		t.Fatalf("expected HTTP_ADDR port error, got %v", err)
	}
}

func TestParseDistributedModuleSelection(t *testing.T) {
	cfg, err := Parse(map[string]string{
		"DATABASE_URL":    "postgres://app@localhost:5432/app?sslmode=disable",
		"ENABLED_MODULES": "notification", "EVENT_BUS": "nats",
		"NATS_URL": "nats://localhost:4222", "IDENTITY_GRPC_ADDR": "identity:9090",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.HasModule("notification") || cfg.HasModule("identity") {
		t.Fatalf("unexpected module selection: %q", cfg.EnabledModules)
	}
	_, err = Parse(map[string]string{
		"DATABASE_URL":    "postgres://app@localhost:5432/app?sslmode=disable",
		"ENABLED_MODULES": "notification", "EVENT_BUS": "nats", "NATS_URL": "nats://localhost:4222",
	})
	if err == nil || !strings.Contains(err.Error(), "IDENTITY_GRPC_ADDR") {
		t.Fatalf("expected remote identity address error, got %v", err)
	}
}

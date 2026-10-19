// Package metrics configures OpenTelemetry metrics and Prometheus scraping.
package metrics

import (
	"context"
	"fmt"
	"net/http"

	promclient "github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
)

// New creates a meter provider and a private Prometheus scrape handler.
func New(ctx context.Context, endpoint, serviceName string, insecure bool) (*metric.MeterProvider, http.Handler, error) {
	registry := promclient.NewRegistry()
	promExporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, nil, fmt.Errorf("create Prometheus exporter: %w", err)
	}
	options := []metric.Option{
		metric.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
		metric.WithReader(promExporter),
	}
	if endpoint != "" {
		exportOptions := []otlpmetricgrpc.Option{otlpmetricgrpc.WithEndpoint(endpoint)}
		if insecure {
			exportOptions = append(exportOptions, otlpmetricgrpc.WithInsecure())
		}
		exporter, err := otlpmetricgrpc.New(ctx, exportOptions...)
		if err != nil {
			return nil, nil, fmt.Errorf("create OTLP metrics exporter: %w", err)
		}
		options = append(options, metric.WithReader(metric.NewPeriodicReader(exporter)))
	}
	provider := metric.NewMeterProvider(options...)
	return provider, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), nil
}

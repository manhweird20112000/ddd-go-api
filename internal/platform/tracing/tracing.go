// Package tracing configures OpenTelemetry traces for the process.
package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// New creates a trace provider with optional OTLP export.
func New(ctx context.Context, endpoint, serviceName string, insecure bool) (*sdktrace.TracerProvider, error) {
	options := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.NewSchemaless(attribute.String("service.name", serviceName))),
	}
	if endpoint != "" {
		exportOptions := []otlptracegrpc.Option{otlptracegrpc.WithEndpoint(endpoint)}
		if insecure {
			exportOptions = append(exportOptions, otlptracegrpc.WithInsecure())
		}
		exporter, err := otlptracegrpc.New(ctx, exportOptions...)
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	return sdktrace.NewTracerProvider(options...), nil
}

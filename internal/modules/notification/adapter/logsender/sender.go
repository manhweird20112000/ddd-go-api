// Package logsender implements a sample welcome sender using structured logs.
package logsender

import (
	"context"
	"log/slog"

	"go.opentelemetry.io/otel/trace"

	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Sender logs a welcome action for the example template.
type Sender struct{ logger *slog.Logger }

// New creates a logging welcome sender.
func New(logger *slog.Logger) *Sender { return &Sender{logger: logger} }

// SendWelcome records delivery without exposing an email address in logs.
func (s *Sender) SendWelcome(ctx context.Context, _ string) error {
	s.logger.InfoContext(ctx, "welcome_sent",
		"request_id", httpx.RequestID(ctx),
		"trace_id", trace.SpanContextFromContext(ctx).TraceID().String())
	return nil
}

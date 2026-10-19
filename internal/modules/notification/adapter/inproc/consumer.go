// Package inproc subscribes notification to local identity events.
package inproc

import (
	"context"
	"encoding/json"
	"fmt"

	"go.opentelemetry.io/otel/propagation"

	"github.com/yourorg/go-template/internal/modules/identity/contract"
	"github.com/yourorg/go-template/internal/modules/notification/domain"
	"github.com/yourorg/go-template/internal/platform/eventbus"
	"github.com/yourorg/go-template/internal/platform/httpx"
)

// Service handles registration events independent of the transport envelope.
type Service interface {
	HandleRegistered(context.Context, domain.RegisteredUser) error
}

// Subscribe attaches the notification consumer to the local bus.
func Subscribe(bus *eventbus.InProcess, service Service) error {
	return bus.Subscribe(contract.UserRegisteredType, func(ctx context.Context, event eventbus.Event) error {
		if event.Version != 1 {
			return fmt.Errorf("unsupported registration event version %d", event.Version)
		}
		var payload contract.UserRegisteredV1
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return fmt.Errorf("decode registration event: %w", err)
		}
		if payload.UserID == "" || payload.UserID != event.AggregateID {
			return fmt.Errorf("registration payload mismatch: %w", domain.ErrInvalidEvent)
		}
		ctx = httpx.WithRequestID(ctx, event.RequestID)
		ctx = propagation.TraceContext{}.Extract(ctx, propagation.MapCarrier{"traceparent": event.Traceparent})
		return service.HandleRegistered(ctx, domain.RegisteredUser{EventID: event.ID, UserID: payload.UserID})
	})
}

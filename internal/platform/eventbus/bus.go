// Package eventbus defines event delivery and an in-process implementation.
package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Event is the transport envelope persisted in a module outbox.
type Event struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     int             `json:"version"`
	OccurredAt  time.Time       `json:"occurred_at"`
	AggregateID string          `json:"aggregate_id"`
	RequestID   string          `json:"request_id,omitempty"`
	TraceID     string          `json:"trace_id,omitempty"`
	Traceparent string          `json:"traceparent,omitempty"`
	Payload     json.RawMessage `json:"payload"`
}

// Publisher sends an event to registered consumers.
type Publisher interface {
	Publish(context.Context, Event) error
}

// Handler processes one event. It must be idempotent across redelivery.
type Handler func(context.Context, Event) error

// ErrNoSubscriber means the selected transport has no consumer for this event.
var ErrNoSubscriber = errors.New("no event subscriber")

// InProcess delivers events synchronously to local handlers.
type InProcess struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
}

// NewInProcess creates an empty local bus.
func NewInProcess() *InProcess {
	return &InProcess{handlers: make(map[string][]Handler)}
}

// Subscribe registers a handler for an exact event type.
func (b *InProcess) Subscribe(eventType string, handler Handler) error {
	if eventType == "" || handler == nil {
		return errors.New("event type and handler are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
	return nil
}

// Publish invokes all matching handlers and returns the first failure.
func (b *InProcess) Publish(ctx context.Context, event Event) error {
	b.mu.RLock()
	handlers := append([]Handler(nil), b.handlers[event.Type]...)
	b.mu.RUnlock()
	if len(handlers) == 0 {
		return fmt.Errorf("%s: %w", event.Type, ErrNoSubscriber)
	}
	for _, handler := range handlers {
		if err := handler(ctx, event); err != nil {
			return fmt.Errorf("handle %s: %w", event.Type, err)
		}
	}
	return nil
}

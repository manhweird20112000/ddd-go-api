package eventbus

import (
	"context"
	"errors"
	"testing"
)

func TestInProcessDispatch(t *testing.T) {
	bus := NewInProcess()
	if err := bus.Publish(context.Background(), Event{Type: "UserRegistered"}); err == nil {
		t.Fatal("expected error without subscribers")
	}
	called := 0
	if err := bus.Subscribe("UserRegistered", func(_ context.Context, event Event) error {
		if event.AggregateID != "user-1" {
			t.Fatalf("unexpected aggregate: %s", event.AggregateID)
		}
		called++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), Event{Type: "UserRegistered", AggregateID: "user-1"}); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("handler called %d times", called)
	}
	expected := errors.New("consumer failed")
	if err := bus.Subscribe("Broken", func(context.Context, Event) error { return expected }); err != nil {
		t.Fatal(err)
	}
	if err := bus.Publish(context.Background(), Event{Type: "Broken"}); !errors.Is(err, expected) {
		t.Fatalf("expected wrapped handler failure, got %v", err)
	}
}

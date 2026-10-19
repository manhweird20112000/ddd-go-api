package eventbus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

const (
	streamName   = "MODULE_EVENTS"
	consumerName = "notification_welcome_v1"
)

// NATS publishes events and consumes them through a durable JetStream consumer.
type NATS struct {
	conn     *nats.Conn
	js       jetstream.JetStream
	consumer jetstream.Consumer
	logger   *slog.Logger
}

// NewNATS opens JetStream and ensures the event stream and consumer exist.
func NewNATS(ctx context.Context, url string, logger *slog.Logger) (*NATS, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if logger == nil {
		return nil, errors.New("NATS logger is required")
	}
	conn, err := nats.Connect(url, nats.Timeout(3*time.Second), nats.MaxReconnects(-1))
	if err != nil {
		return nil, fmt.Errorf("connect NATS: %w", err)
	}
	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("open JetStream: %w", err)
	}
	if _, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name: streamName, Subjects: []string{"events.>", "dead.>"},
		Storage: jetstream.FileStorage, Duplicates: 2 * time.Minute, MaxAge: 30 * 24 * time.Hour,
	}); err != nil {
		conn.Close()
		return nil, fmt.Errorf("ensure JetStream event stream: %w", err)
	}
	consumer, err := js.CreateOrUpdateConsumer(ctx, streamName, jetstream.ConsumerConfig{
		Durable: consumerName, AckPolicy: jetstream.AckExplicitPolicy,
		FilterSubject: "events.identity.user_registered", AckWait: 30 * time.Second,
		MaxDeliver: 10, MaxAckPending: 100,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("ensure JetStream consumer: %w", err)
	}
	return &NATS{conn: conn, js: js, consumer: consumer, logger: logger}, nil
}

// Publish stores an envelope in JetStream and waits for server confirmation.
func (n *NATS) Publish(ctx context.Context, event Event) error {
	if event.ID == "" || event.Type == "" {
		return errors.New("event ID and type are required")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode event envelope: %w", err)
	}
	msg := &nats.Msg{Subject: "events." + event.Type, Header: nats.Header{}, Data: data}
	msg.Header.Set(nats.MsgIdHdr, event.ID)
	if _, err := n.js.PublishMsg(ctx, msg); err != nil {
		return fmt.Errorf("publish event to JetStream: %w", err)
	}
	return nil
}

// RunConsumer pulls messages until cancellation and acknowledges after success.
func (n *NATS) RunConsumer(ctx context.Context, handler Handler) error {
	if handler == nil {
		return errors.New("JetStream handler is required")
	}
	for ctx.Err() == nil {
		batch, err := n.consumer.Fetch(1, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			n.logger.ErrorContext(ctx, "fetch JetStream event", "error", err)
			continue
		}
		for msg := range batch.Messages() {
			if err := n.handleMessage(ctx, msg, handler); err != nil {
				n.logger.ErrorContext(ctx, "handle JetStream event", "error", err)
			}
		}
		if err := batch.Error(); err != nil && !errors.Is(err, jetstream.ErrNoMessages) && ctx.Err() == nil {
			n.logger.ErrorContext(ctx, "receive JetStream batch", "error", err)
		}
	}
	return nil
}

func (n *NATS) handleMessage(ctx context.Context, msg jetstream.Msg, handler Handler) error {
	var event Event
	if err := json.Unmarshal(msg.Data(), &event); err != nil {
		return n.retryOrDead(ctx, msg, fmt.Errorf("decode event envelope: %w", err))
	}
	if err := handler(ctx, event); err != nil {
		return n.retryOrDead(ctx, msg, err)
	}
	if err := msg.Ack(); err != nil {
		return fmt.Errorf("ack event %s: %w", event.ID, err)
	}
	return nil
}

func (n *NATS) retryOrDead(ctx context.Context, msg jetstream.Msg, cause error) error {
	metadata, err := msg.Metadata()
	if err != nil {
		return errors.Join(cause, fmt.Errorf("read event delivery metadata: %w", err))
	}
	if metadata.NumDelivered >= 10 {
		dead := &nats.Msg{Subject: "dead.identity.user_registered", Header: nats.Header{}, Data: msg.Data()}
		dead.Header.Set(nats.MsgIdHdr, fmt.Sprintf("dead-%d-%d", metadata.Sequence.Stream, metadata.Sequence.Consumer))
		if _, err := n.js.PublishMsg(ctx, dead); err != nil {
			return errors.Join(cause, fmt.Errorf("publish dead-letter event: %w", err))
		}
		if err := msg.Term(); err != nil {
			return errors.Join(cause, fmt.Errorf("terminate dead event: %w", err))
		}
		n.logger.ErrorContext(ctx, "JetStream event dead", "stream_sequence", metadata.Sequence.Stream)
		return cause
	}
	delay := time.Duration(1<<min(metadata.NumDelivered, 8)) * time.Second
	if err := msg.NakWithDelay(delay); err != nil {
		return errors.Join(cause, fmt.Errorf("retry JetStream event: %w", err))
	}
	return cause
}

// Ready verifies the broker is connected and JetStream responds within ctx.
func (n *NATS) Ready(ctx context.Context) error {
	if !n.conn.IsConnected() {
		return errors.New("NATS is disconnected")
	}
	_, err := n.js.AccountInfo(ctx)
	return err
}

// Close stops the NATS connection after consumers and relay have drained.
func (n *NATS) Close() { n.conn.Close() }

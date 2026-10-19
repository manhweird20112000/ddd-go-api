CREATE SCHEMA IF NOT EXISTS notification;

CREATE TABLE notification.processed_events (
    event_id uuid NOT NULL,
    consumer text NOT NULL,
    processed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (event_id, consumer)
);

CREATE TABLE notification.welcome_deliveries (
    event_id uuid PRIMARY KEY,
    user_id uuid NOT NULL,
    recipient text NOT NULL,
    sent_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE notification.outbox (
    id uuid PRIMARY KEY,
    event_type text NOT NULL,
    version integer NOT NULL,
    aggregate_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL,
    request_id text NOT NULL DEFAULT '',
    trace_id text NOT NULL DEFAULT '',
    traceparent text NOT NULL DEFAULT '',
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'published', 'dead')),
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    last_error text NOT NULL DEFAULT ''
);

CREATE INDEX notification_outbox_pending_idx ON notification.outbox (next_attempt_at, occurred_at, id)
WHERE status = 'pending';

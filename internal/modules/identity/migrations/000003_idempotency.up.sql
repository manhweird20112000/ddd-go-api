CREATE TABLE identity.idempotency_keys (
    scope text NOT NULL,
    key text NOT NULL,
    request_hmac bytea NOT NULL,
    status_code integer,
    content_type text NOT NULL DEFAULT 'application/json',
    nonce bytea,
    ciphertext bytea,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (scope, key)
);

CREATE INDEX idempotency_expiry_idx ON identity.idempotency_keys (expires_at);

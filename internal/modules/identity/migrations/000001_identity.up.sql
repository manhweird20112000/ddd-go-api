CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.users (
    id uuid PRIMARY KEY,
    email text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX users_cursor_idx ON identity.users (created_at, id);

CREATE TABLE identity.refresh_tokens (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES identity.users(id) ON DELETE CASCADE,
    family_id uuid NOT NULL,
    token_hash bytea NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    used_at timestamptz,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_family_idx ON identity.refresh_tokens (family_id);
CREATE INDEX refresh_tokens_user_idx ON identity.refresh_tokens (user_id);

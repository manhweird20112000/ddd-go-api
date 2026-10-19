-- name: CreateUser :one
INSERT INTO identity.users (id, email, password_hash)
VALUES ($1, $2, $3)
RETURNING id, email, password_hash, created_at, updated_at;

-- name: GetUser :one
SELECT id, email, password_hash, created_at, updated_at
FROM identity.users
WHERE id = $1;

-- name: FindUserByEmail :one
SELECT id, email, password_hash, created_at, updated_at
FROM identity.users
WHERE email = $1;

-- name: ListFirstUsers :many
SELECT id, created_at
FROM identity.users
ORDER BY created_at, id
LIMIT $1;

-- name: ListUsersAfter :many
SELECT id, created_at
FROM identity.users
WHERE (created_at, id) > (sqlc.arg(after_at)::timestamptz, sqlc.arg(after_id)::uuid)
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit);

-- name: InsertRefreshToken :exec
INSERT INTO identity.refresh_tokens (id, user_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5);

-- name: GetRefreshTokenForUpdate :one
SELECT id, user_id, family_id, token_hash, expires_at, used_at, revoked_at, created_at
FROM identity.refresh_tokens
WHERE token_hash = $1
FOR UPDATE;

-- name: MarkRefreshTokenUsed :exec
UPDATE identity.refresh_tokens
SET used_at = now()
WHERE id = $1;

-- name: RevokeRefreshFamily :exec
UPDATE identity.refresh_tokens
SET revoked_at = now()
WHERE family_id = $1 AND revoked_at IS NULL;

-- name: InsertOutboxEvent :exec
INSERT INTO identity.outbox (
    id, event_type, version, aggregate_id, occurred_at,
    request_id, trace_id, traceparent, payload
) VALUES (
    sqlc.arg(id), sqlc.arg(event_type), sqlc.arg(version), sqlc.arg(aggregate_id),
    sqlc.arg(occurred_at), sqlc.arg(request_id), sqlc.arg(trace_id),
    sqlc.arg(traceparent), sqlc.arg(payload)
);

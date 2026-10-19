-- name: MarkEventProcessed :one
INSERT INTO notification.processed_events (event_id, consumer)
VALUES ($1, $2)
ON CONFLICT DO NOTHING
RETURNING event_id;

-- name: InsertWelcomeDelivery :exec
INSERT INTO notification.welcome_deliveries (event_id, user_id, recipient)
VALUES ($1, $2, $3);

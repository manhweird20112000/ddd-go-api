# ADR 0002: Transactional outbox with at-least-once delivery

Status: accepted, 2026-10-19.

Identity writes `UserRegisteredV1` to its outbox in the same PostgreSQL transaction as a new user. A relay claims pending rows with `FOR UPDATE SKIP LOCKED`, publishes to the selected bus, and marks them published. This prevents losing an event after a successful business commit and permits concurrent relays.

A crash after publish and before the outbox update can redeliver an event. Notification therefore writes `(event_id, consumer)` and its delivery row in one transaction. External senders must also support a stable idempotency key for exactly-once effects; the sample logging sender only demonstrates the port.

# Extract Notification into a separate service

1. Run `make fmt lint test build`, `make arch-check`, `make coverage`, and `make test-integration` before changing deployment. Keep `notification/app` and `notification/domain` unchanged.
2. Set `ENABLED_MODULES=identity` and `EVENT_BUS=nats` for Identity, and `ENABLED_MODULES=notification` plus `EVENT_BUS=nats` for the Notification worker. Point both at the same authenticated JetStream cluster. Verify a registration reaches Notification.
3. Configure `IDENTITY_GRPC_ADDR` for Notification to call Identity's remote contract adapter. Protect gRPC with TLS and service authentication before moving across an untrusted network. Confirm the shared contract suite passes for local and remote adapters.
4. Move `notification` schema and migration history to a dedicated PostgreSQL database. Run its migrations with a migration role, then grant a Notification-only runtime role. Copy data with a planned replication/cutover window. Set `NOTIFICATION_DATABASE_URL`; remove Notification's access to the original database after cutover.
5. Deploy the Notification worker independently. Route only its JetStream durable consumer to the new process. Stop the old in-process consumer before cutover, then verify processed markers and deliveries with a duplicate event probe.
6. Move operational ownership of Notification's relay, retention, metrics, and alerts to the new deployment. Check retry, dead-letter, and shutdown behavior under failure.
7. If splitting repositories, move `internal/platform` into a versioned shared Go module or copy only the stable transport primitives. Keep business contracts versioned and avoid importing Identity's implementation.
8. Remove Notification from the original process's `ENABLED_MODULES`, its database URL, and its deployment permissions. Run both process modes and the contract, architecture, integration, and distributed smoke tests again.

The example logging Sender has no externally durable delivery guarantee. Replace it with a provider adapter that accepts an idempotency key before treating a welcome notification as exactly once.

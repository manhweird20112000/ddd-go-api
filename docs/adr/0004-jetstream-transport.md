# ADR 0004: JetStream for distributed event transport

Status: accepted, 2026-10-19.

The default deployment uses an in-process event bus. Distributed deployment selects NATS JetStream with a durable consumer, explicit acknowledgement after processing, delayed retries, and a dead-letter subject. The stream retains messages for 30 days; processed markers remain for 45 days so replayed events remain idempotent within that window.

The outbox remains the source of committed events in both deployments. JetStream message IDs reduce duplicates during its configured deduplication window but do not replace consumer idempotency. Broker authorization and multi-node persistence must be added for production infrastructure.

# Phase 4 Notification Plan

**Goal:** Consume `identity.user_registered` and record one welcome delivery even with duplicate events or concurrent workers.

## Design

- Notification owns its `notification` schema, migration history, `processed_events`, `welcome_deliveries`, and outbox tables.
- Its application service has a small repository port, unit-of-work port, and Sender port. It inserts a `(event_id, consumer)` marker and delivery row in one transaction.
- An in-process adapter parses the public identity event payload; the application does not import the identity module or transport package.
- The sample Sender logs the user ID in Phase 4. Phase 5 changes the recipient to an email fetched through `identity.contract.Service`.

## Steps and gates

1. Add notification schema, module migration runner, SQL queries, and generated pgx code.
2. Implement application consumer with unit tests for first delivery, duplicate suppression, and sender failure.
3. Wire in-process subscription into API and worker, then run PostgreSQL integration with duplicate events and two concurrent workers.
4. Run `make fmt lint test build`, `make test-integration`, and Compose registration smoke. Update progress and commit one Conventional Commit.

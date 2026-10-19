# Phase 6 Distributed Transport Plan

**Goal:** Run Identity and Notification in separate processes without changing their application or domain logic.

## Design

- Generate Go and gRPC code from `api/proto/identity/v1/identity.proto` with buf. Identity's gRPC server adapts `contract.Service`; its client implements the same contract with a default deadline, retries for idempotent `GetUser`, jitter, and a circuit breaker.
- NATS JetStream adapts `eventbus.Publisher` and runs a durable explicit-ack consumer. The identity outbox relay publishes to NATS; notification acknowledges only after its database transaction succeeds.
- `ENABLED_MODULES` selects local Identity and Notification. The composition root resolves a local identity contract when Identity is enabled, otherwise a gRPC client at `IDENTITY_GRPC_ADDR`.
- Default Compose remains one-process in-process mode. The distributed profile adds a separate identity API and notification worker with JetStream. The default service may remain running in that profile due Compose profile semantics; the distributed smoke targets the isolated identity API port.

## Steps and gates

1. Add proto/buf and generated code, then gRPC server/client adapters with shared contract tests.
2. Add JetStream publisher and durable consumer; configure retries and broker readiness.
3. Add registry and module selection to API/worker, then distributed Compose services.
4. Run `make fmt lint test build`, `make proto`, `make test-integration`, local and distributed Compose smoke. Update progress and commit one Conventional Commit.

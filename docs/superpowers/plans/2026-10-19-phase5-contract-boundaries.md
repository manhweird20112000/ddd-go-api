# Phase 5 Contracts and Boundaries Plan

**Goal:** Notification resolves a registration's email through `identity.contract.Service`, while static checks enforce module and layer imports.

## Design

- Identity publishes only `user_id`; its public contract exposes `GetUser(ctx, id)` with a DTO containing ID, email, and creation time.
- Identity's in-process adapter wraps the application query. Notification consumes a small lookup port and an adapter wraps the public identity contract. The composition root injects this local contract.
- Notification stores the resolved email as delivery recipient, but the sample sender logs only the user ID to avoid PII in logs.
- An architecture test loads production packages via `go/packages` and rejects module-to-module imports beyond `contract`, external dependencies in domains, platform imports from modules, and infrastructure imports in application packages.

## Steps and gates

1. Add identity service contract and local adapter with shared behavior tests.
2. Extend notification lookup and integration tests to verify email resolution and one delivery on redelivery.
3. Add architecture test and depguard configuration; prove a deliberate forbidden import fails the checker.
4. Run `make fmt lint test build`, `make arch-check`, integration tests, and Compose smoke. Update progress and commit one Conventional Commit.

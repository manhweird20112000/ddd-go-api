# ADR 0003: Contract-only module dependencies and separate schemas

Status: accepted, 2026-10-19.

Identity and Notification own separate PostgreSQL schemas, migration histories, and runtime roles. Composition opens a pool for each enabled module. A module may import another module's `contract` package but cannot import its domain, application, or adapters. The architecture test and depguard enforce import rules; PostgreSQL privileges enforce data access in Compose.

Events carry IDs instead of copied user email. Notification resolves the email through Identity's contract. This adds a synchronous dependency for welcome delivery, so the client uses a deadline, bounded retries for the idempotent lookup, and a circuit breaker. Extraction can change the contract adapter without changing Notification's application logic.

# Repository architecture rules

- Run `make fmt lint test build` before declaring a phase complete. Also run `make arch-check`, `make coverage`, and relevant integration tests when changing module boundaries or persistence.
- Each module owns `domain`, `app`, `adapter`, `migrations`, a PostgreSQL schema, and a runtime database role. Domain packages import only Go's standard library. Application packages define small consumer-owned ports and never import adapters, SQL drivers, transport, or platform infrastructure.
- Cross-module imports may target only the other module's `contract` package. Never join tables or create foreign keys across module schemas. Pass IDs in events and resolve required data through a contract.
- Put infrastructure and process wiring in `internal/platform` and `internal/app` respectively. The platform cannot import a business module.
- Keep a business write and its outbox row in one transaction. Consumers insert a processed marker and their database effect in one transaction. Design for at-least-once delivery.
- Add migrations in the owning module, then run `make sqlc`. Register new modules through `make new-module name=billing`; add a dedicated runtime role before production deployment.
- Do not log passwords, refresh tokens, signing keys, or user email. Keep secrets in environment variables; `.env` is local and ignored by Git.
- Use Go 1.26.9 or newer security-patched 1.26 toolchain. Check real library signatures with `go doc` after dependency changes.

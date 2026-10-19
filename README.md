# Go modular backend template

This repository is a runnable Go 1.26.9 backend template with two example bounded contexts. Identity handles registration, authentication, refresh rotation, and user reads. Notification consumes `UserRegisteredV1`, resolves the user through an Identity contract, and records one welcome delivery per event. Business code stays in `domain` and `app`; HTTP, PostgreSQL, in-process events, gRPC, and JetStream are adapters.

## Start locally

Install Go 1.26.9 or allow Go's automatic toolchain download, GNU Make, and Docker Compose. Buf is needed for protobuf generation. On a fresh checkout:

```sh
make dev-env
docker compose up --build -d
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

`make dev-env` creates an ignored `.env` with random PostgreSQL passwords, an Ed25519 private key, and an idempotency encryption key. It refuses to replace an existing `.env`. Compose runs PostgreSQL 16, Redis, an OpenTelemetry collector, schema-role provisioning, migrations, and the API. `/healthz` checks liveness; `/readyz` checks enabled dependencies. Prometheus metrics are on `http://localhost:9100/metrics`.

From PowerShell, exercise registration, login, and an authenticated read without printing tokens:

```powershell
$email = "demo-$([guid]::NewGuid().ToString('N'))@example.test"
$password = 'correct-password-123'
$body = @{ email = $email; password = $password } | ConvertTo-Json -Compress
$registerKey = "register-$([guid]::NewGuid().ToString('N'))"
$registered = Invoke-RestMethod -Uri 'http://localhost:8080/v1/users/register' -Method Post -ContentType 'application/json' -Headers @{ 'Idempotency-Key' = $registerKey } -Body $body
$loginKey = "login-$([guid]::NewGuid().ToString('N'))"
$login = Invoke-RestMethod -Uri 'http://localhost:8080/v1/auth/login' -Method Post -ContentType 'application/json' -Headers @{ 'Idempotency-Key' = $loginKey } -Body $body
$me = Invoke-RestMethod -Uri 'http://localhost:8080/v1/users/me' -Headers @{ Authorization = "Bearer $($login.access_token)" }
$me.id -eq $registered.id
```

All three Identity POST routes require an `Idempotency-Key` containing 8–128 letters, digits, `-`, `_`, `.`, or `:`. A matching request and key replay the same response for 24 hours; a changed request returns 409. Rate limiting allows 20 writes per minute per client IP and route. The API rejects request bodies larger than 1 MiB.

## Distributed mode

The same code runs Identity and Notification in separate processes. The outbox publishes to NATS JetStream and Notification uses Identity's gRPC contract:

```sh
docker compose --profile distributed up --build -d --scale api=0
curl http://localhost:8081/readyz
docker compose --profile distributed logs notification-worker
```

Use port 8081 in the PowerShell example above to register in distributed mode. `--scale api=0` stops the unprofiled monolith API that Compose otherwise includes with the distributed profile. `docker compose --profile distributed down` stops the stack without deleting database data.

## Structure and contracts

| Path | Responsibility |
| --- | --- |
| `internal/modules/identity` | Identity domain, application, adapters, contract, schema migrations |
| `internal/modules/notification` | Notification consumer, sender port, adapters, schema migrations |
| `internal/platform` | Configuration, auth, HTTP, database, event bus, outbox, tracing, metrics |
| `internal/app` | Composition root and static module registry |
| `cmd/api`, `cmd/worker`, `cmd/migrate` | Process entrypoints |
| `api/proto` | Versioned Identity gRPC API |

`identity.contract.Service.GetUser` is the synchronous cross-module API. `identity.user_registered` version 1 is the event; its payload contains only `user_id`. Identity owns `users`, `refresh_tokens`, `outbox`, and `idempotency_keys`. Notification owns `processed_events`, `welcome_deliveries`, and `outbox`. Runtime roles cannot read each other's schema. See [architecture](docs/architecture.md), [ADRs](docs/adr), and the [extraction checklist](docs/extract-to-microservice.md).

| Method | Route | Success | Common errors |
| --- | --- | --- | --- |
| POST | `/v1/users/register` | 201 user ID, email, created time | 400, 409, 413, 429 |
| POST | `/v1/auth/login` | 200 access and refresh token pair | 400, 401, 429 |
| POST | `/v1/auth/refresh` | 200 rotated token pair | 400, 401, 429 |
| GET | `/v1/users/me` | 200 current user | 401, 404 |
| GET | `/v1/users?limit=20` | 200 page and next cursor | 400 |
| GET | `/healthz`, `/readyz` | 200 liveness or readiness | 503 readiness |

POST bodies are JSON: registration and login use `{"email":"demo@example.test","password":"correct-password-123"}`. Refresh sends the `refresh_token` returned by login. The `Authorization: Bearer` header protects `/v1/users/me` and `/v1/users`. An error example is `{"error":{"code":"unauthorized","message":"invalid credentials or token","request_id":"request-123"}}`.

## Extend and verify

```sh
make new-module name=billing
make sqlc
make fmt lint test build arch-check
make coverage
make test-integration
make test-race
make vulncheck
```

The generator creates a `billing` domain, application port and service, PostgreSQL and HTTP adapters, migration, sqlc configuration, sample test, and registry entry. Its read-only `/v1/billing/status` endpoint becomes available when `ENABLED_MODULES` includes `billing` and the migration has run. The generated module uses `DATABASE_URL` locally unless `BILLING_DATABASE_URL` is set. Before production use, provision a separate `billing_app` role, grant only the `billing` schema, and add that URL to deployment configuration. Add a durable outbox, idempotency handling, and consumer tests when adding business writes or events.

CI runs formatting, lint, race tests, integration tests, architecture checks, per-package 80% domain/application coverage, govulncheck, Buf lint and PR breaking checks, code generation comparison, Go build, and Docker build. `make proto` regenerates gRPC files. `make down` stops the default Compose services; it does not remove volumes.

## Current limits

- The logging welcome sender is illustrative. A real provider needs its own idempotency key to avoid duplicate external deliveries after a crash.
- Compose uses single-node PostgreSQL, Redis, and NATS without production TLS, broker authorization, backup, or high availability. Add those before exposing this deployment publicly. gRPC is plaintext inside the Compose network.
- Rate limiting uses the TCP peer address. Configure a trusted proxy strategy before placing replicas behind a reverse proxy; otherwise all clients behind that proxy share a bucket.
- Rotating the idempotency encryption key invalidates stored replay responses; plan a 24-hour overlap or drain before rotating. JWT signing key rotation needs a verifier key rollout.
- The generated module is a read-only starting point. It does not automatically add a production database role or a new event consumer.

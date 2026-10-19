# Phase 8 Tooling and Documentation Plan

**Goal:** Complete the reusable module scaffolder, CI quality gates, architecture guidance, ADRs, and operator README.

## Design decisions

- A static registry lists built-in and generated modules. The generator edits two marked sections in one registry file and creates runnable domain, application, HTTP/Postgres adapters, migration, module wiring, and a test. Generated modules have a read-only status endpoint and their own schema; deployment owners provision a separate role before enabling one in production.
- Configuration accepts syntactically valid module names and optional `<MODULE>_DATABASE_URL`. The composition root rejects unregistered modules, opens their pools, registers routes, and includes their pools in readiness.
- CI runs format, lint, architecture, race, coverage, vulnerability, Buf lint/breaking, integration, and Docker build. Coverage gate measures the combined domain and application packages.
- Docs describe commands, deployment modes, contracts, events, schema ownership, known limitations, and the extraction path.

## Steps and gates

1. Implement registry, dynamic configuration, generator, and a generated `billing` probe that passes build/lint/architecture; remove only the probe afterward.
2. Add CI gates and coverage tooling; raise domain/application coverage to 80% where needed.
3. Add `.cursor` rules, AGENTS.md, four remaining ADRs, extraction checklist, and README.
4. Run `make fmt lint test build`, race, coverage, architecture, integration, Buf, vulnerability scan, Docker build, both Compose modes, and distributed end-to-end smoke. Update progress and create one Conventional Commit.

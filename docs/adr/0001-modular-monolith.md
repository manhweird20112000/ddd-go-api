# ADR 0001: Modular monolith with extractable modules

Status: accepted, 2026-10-19.

Each bounded context owns its domain, application, adapters, migrations, database schema, and role. Other modules import only its contract. The composition root selects in-process or remote adapters. This keeps the default deployment simple while allowing a module to move to another process without changing its business logic.

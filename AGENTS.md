# Agent rules for datatug/datatug-core

## Core is engine-free: database access goes through DALgo adapters

datatug-core holds DataTug's engine-neutral logic. It talks to databases only through
DALgo's interfaces (`github.com/dal-go/dalgo`). Anything specific to one database engine
lives in that engine's DALgo adapter (`dal-go/dalgo2<engine>`, for example
`dal-go/dalgo2postgres`), never here. That covers:

- database drivers (pgx, lib/pq, go-sql-driver/mysql, sqlite, mssql, mongo, …) and
  `database/sql`;
- engine-specific SQL and catalog queries (`pg_catalog`, `pg_roles`,
  `information_schema`, `sqlite_master`, `sys.objects`, …), including SQL kept as
  string constants "for an adapter to run later";
- schema, privilege, role or any other introspection of a particular engine.

When a feature needs something an engine knows (its schema, its roles and grants, its
statistics), the work is:

1. propose an optional, engine-neutral capability interface in DALgo (`dbschema` or the
   fitting package; DALgo's own `AGENTS.md` requires a human's explicit approval for
   new exported API);
2. implement it in the engine's `dalgo2<engine>` adapter, with its integration tests;
3. consume the interface here.

Founder, 2026-11-04: "Core is not supposed to have Postgres driver … if we need to read
Postgres schema, that should be done in Dalgo adapter, not custom something." This rule
exists because an agent once wrote Postgres catalog SQL into this repository.

Enforced by `internal/architecture/engine_free_test.go`: it fails when `go.mod` requires
a known database driver, or when non-test Go code imports `database/sql` or contains
engine catalog names. Do not weaken the test to get a change in; move the code to an
adapter.

# Agent rules for datatug/datatug-core

## Core is engine-free: database access goes through DALgo adapters

datatug-core holds DataTug's engine-neutral logic. It talks to databases only through
DALgo's interfaces (`github.com/dal-go/dalgo`). Anything specific to one database engine
lives in that engine's DALgo adapter (`dal-go/dalgo2<engine>`, for example
`dal-go/dalgo2postgres`), never here. That covers:

- database drivers (pgx, lib/pq, go-sql-driver/mysql, sqlite, mssql, mongo, …) and
  `database/sql`;
- engine-specific SQL and catalog queries (any `pg_*` catalog, `information_schema`,
  `sqlite_master`, `PRAGMA`, `sys.*`, `mysql.user`, Oracle `dba_*`/`v$*`, …), including SQL
  kept as string constants "for an adapter to run later", in `.sql` files or in C sources.
  This is stricter than DALgo: even a Go comment here may not name an engine catalog;
- schema, privilege, role or any other introspection of a particular engine.

When a feature needs something an engine knows (its schema, its roles and grants, its
statistics), the work is:

1. use DALgo's `dbschema.SchemaReader` and its existing optional interfaces if they
   already express the need;
2. otherwise propose extending `dbschema` with a new optional, engine-neutral interface
   (DALgo's own `AGENTS.md` requires a human's explicit approval for new exported API);
3. implement it in the engine's `dalgo2<engine>` adapter, with its integration tests;
4. consume the interface here.

Founder, 2026-11-04: "Core is not supposed to have Postgres driver … if we need to read
Postgres schema, that should be done in Dalgo adapter, not custom something." This rule
exists because an agent once wrote Postgres catalog SQL into this repository.

Enforced by `internal/architecture/engine_free_test.go`: it fails when any go.mod outside
`testdata/` requires a known database driver, or when non-test Go code, `.sql` or C files
import `database/sql` or contain engine catalog names. `testdata/` fixtures (for example
the storage spike's sqlite module) are exempt. Do not weaken the test to get a change in; move the code to an
adapter.

No Go file here, tests included, imports a separate adapter module (`github.com/dal-go/dalgo2*`); adapters
bundled in the DALgo core module (`github.com/dal-go/dalgo/adapters/dalgo2memory`, `.../dalgo2fs`)
are fine, and `internal/architecture/no_adapter_imports_test.go` enforces it (founder, 2026-11-04:
"We are OK to import from core but not from other modules/repos").

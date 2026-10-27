# DataTug Core

[![Build & Test](https://github.com/datatug/datatug-core/actions/workflows/ci.yml/badge.svg)](https://github.com/datatug/datatug-core/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSES/Apache-2.0.txt)
[![Go Report Card](https://goreportcard.com/badge/github.com/datatug/datatug-core)](https://goreportcard.com/report/github.com/datatug/datatug-core)
[![GoDoc](https://godoc.org/github.com/datatug/datatug-core?status.svg)](https://pkg.go.dev/github.com/datatug/datatug-core)

**DataTug Core** is the foundational Go engine and domain library powering the DataTug ecosystem for database schema management, semantic modeling, recordset comparisons, and collaborative data exploration.

---

## Architecture Summary

DataTug provides an open, modular data modeling and schema governance platform designed to bridge technical database schemas with semantic business meaning:

```
                       +---------------------------+
                       |    DataTug Applications   |
                       |  (CLI, Web UI, Extensions)|
                       +-------------+-------------+
                                     |
               +---------------------+---------------------+
               |                                           |
+--------------v---------------+            +--------------v---------------+
|    Semantic & Comparison     |            |      Storage & Drivers       |
|  - meaninggraph (Resolver)   |            |  - filestore (Local Git/FS)  |
|  - recordsetcompare (Diffs)  |            |  - ingitdbschema (Git DBs)   |
|  - schemer (Introspection)   |            |  - dalgostore (DALgo backend)|
|  - detective (Investigations)|            |  - dtprojcreator             |
+--------------+---------------+            +--------------+---------------+
               |                                           |
               +---------------------+---------------------+
                                     |
                       +-------------v-------------+
                       |      Core Data Types      |
                       |  - pkg/datatug            |
                       |  - pkg/dto, pkg/dtconfig  |
                       +-------------+-------------+
                                     |
                       +-------------v-------------+
                       |    Entitlement Contracts  |
                       |  - commercial/licensing   |
                       +---------------------------+
```

### Core Architectural Pillars
- **Meaning Graph**: Resolves data catalog selections, semantic models, and versioned entity relationships while preventing traversal attacks through strict fixture boundaries.
- **Recordset Comparison**: High-performance row and cell-level diffing between query results and tabular snapshots, available natively and compilable to WebAssembly (`cmd/recordsetcompare-wasm`).
- **in-Git Schema Governance**: Treat database schemas, queries, and semantic models as version-controlled code artifacts stored directly in Git repositories.
- **Extensible Storage**: Neutral repository abstractions backed by filesystem (`filestore`) or multi-database drivers (`dalgostore`).
- **Clean Entitlement Boundaries**: Enterprise hooks operate through cleanroom Go interfaces (`commercial/licensing`), keeping community code free from proprietary runtime bloat.

---

## Module Map

| Package / Directory | Purpose |
| :--- | :--- |
| `cmd/recordsetcompare-wasm` | WebAssembly module for running dataset diffs directly in browsers. |
| `pkg/datatug` | Foundational domain entities: Project, Environment, DbServer, Schema, Table, Query. |
| `pkg/meaninggraph` | Semantic graph resolver, catalog integrity checker, and cross-model dependency linker. |
| `pkg/recordsetcompare` | Tabular comparison engine, schema diff calculations, and row change classification. |
| `pkg/recordsetcompare/bridge` | Bridges comparison results into external and serialization formats. |
| `pkg/schemer` | Database schema introspection, table structures, column types, and foreign key relations. |
| `pkg/semantic` | Business taxonomy, semantic types, and entity classification. |
| `pkg/detective` | Investigative query execution, schema consistency checks, and root-cause analysis. |
| `pkg/detectiveai` | AI-assisted schema intelligence and query suggestions. |
| `pkg/storage` | High-level storage contracts and repository persistence abstractions. |
| `pkg/storage/filestore` | Filesystem-backed repository driver for local project trees. |
| `pkg/storage/ingitdbschema`| Git-tracked database schema persistence and version management. |
| `pkg/storage/dalgostore` | Generic database storage backend implemented via DALgo. |
| `pkg/storage/dtprojcreator`| Project scaffolding, directory templates, and project bootstrapping. |
| `pkg/investigation` | Structured data investigation plans, steps, and execution trees. |
| `pkg/incidents` | Incident reports, data quality alerts, and schema regression tracking. |
| `pkg/challenge` | Query challenge runners and regression test benchmarks. |
| `pkg/dbconnection` | Database connection strings, parameter resolution, and connection pooling. |
| `pkg/dto` | Wire format Data Transfer Objects for API and CLI interop. |
| `commercial/licensing` | Enterprise entitlement contracts (`EntitlementVerifier`, `CapacityDescriptor`, `CoordinatorContract`). |

---

## Building and Testing

### Prerequisites
- **Go 1.22+**
- Make or Bash shell

### Build All Modules
```bash
go build ./...
```

### Build WebAssembly Target
To compile the WebAssembly recordset comparator:
```bash
GOOS=js GOARCH=wasm go build -o recordsetcompare.wasm ./cmd/recordsetcompare-wasm
```

### Run Tests
Run the complete test suite across all community packages:
```bash
go test -v ./...
```

Run tests with Go race detector:
```bash
go test -race ./pkg/...
```

Run static analysis:
```bash
go vet ./...
```

---

## Licensing

DataTug is distributed under a **Model A mixed licensing framework**:
- **Community Open Source (Apache 2.0)**: All packages within `pkg/`, `cmd/`, `internal/`, `web/`, `scripts/`, `docs/`, and `spec/` are licensed under the permissive [Apache License, Version 2.0](LICENSES/Apache-2.0.txt).
  ```
  Copyright 2020-2026 DataTug Authors and Contributors (original work Copyright 2020 sneat-team)
  ```
- **Commercial Extensions**: Code and interfaces under `commercial/` are governed by [DataTug Commercial Terms](LICENSES/COMMERCIAL-TERMS.md).

See [`LICENSING.md`](LICENSING.md) for full details on scope boundaries and governance.

---

## Contributing

We warmly welcome community contributions! Please read our [Contributing Guide](CONTRIBUTING.md) and [Code of Conduct](CODE_OF_CONDUCT.md) before submitting pull requests.

Security vulnerabilities should be reported following our [Security Policy](SECURITY.md).

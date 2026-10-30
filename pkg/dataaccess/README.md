# Shared data access boundary

`dataaccess` provides shared DataTug contracts and a bounded read-authorization
foundation. DALgo's `access` package evaluates policies before adapter dispatch.
This package is a core proof checkpoint; production opener wiring, CLI
integration and completion of the data-access-security plan remain pending.

## Consumer contract

The host constructs `NewAuthority` with an authenticated, host-assigned `Subject`,
a resolved `ConnectionRef` and an injected `Resolver`. The subject is fixed for
that authority. Execution input must not choose a different subject or supply a
local-owner flag as authority.

`ConnectionRef` binds a Space, a Space-local protection ID and a fingerprint.
Every resolution must match that subject, Space and ID, the protection ID, and
both connection fingerprints. Fingerprints compare engine, database and engine
identity. When engine identity is absent, the address must match; otherwise the
address is only a hint. An engine identity can identify cluster lineage rather
than a unique physical database.

`Authority.SecureDB` accepts the corresponding DALgo database and returns the
narrow `Database` interface. The host is responsible for verifying that the
supplied database matches the resolved connection and keeping its raw handle out
of consumer code. The factory does not perform native identity discovery or
establish a production opener boundary by itself.

The resolver is a trusted authenticated authority port. It must verify current
membership, protection and grant state, enforce the control plane's grant
lifecycle and duration limits, and return an error when that authority cannot be
established. Client-supplied grants or approval files are not an authority source.
`ServerTime` and `ValidUntil` must be verified server facts, with `ServerTime`
fresh for that resolve call. A cached reply must not be replayed as new authority.
Any future caching adapter must preserve the reply's original age and deadline.
This checkpoint resolves before each operation or reader release and rechecks
successful results before returning them; it provides no caching adapter.

`DecodeAccessRequest` and `DecodeGrant` are the MVP wire admission points. They
reject unknown fields, duplicate or case-folded duplicate keys, trailing input,
and later-phase fields even when null or empty. Connection-read scopes require an
explicit empty `requires` list. Exact-object read requests require
`objects.exact`; request admission does not enable object-grant execution.
Unknown required features are refused. Plain JSON unmarshalling is not wire
admission.

## Supported execution

| Surface | Current behavior |
| --- | --- |
| `Exists` | Checks authority around the typed read. |
| `Get`, `GetMulti` | Stages data into fresh containers; publishes only after successful read and authority recheck. Supports non-nil maps, non-nil pointers and fixed-length slices. Nil, scalar, key-only or inaccessible targets are refused before dispatch. |
| `ExecuteQueryToRecordsReader` | Accepts only DALgo's built-in, unqualified, single-root-collection key-query profile. Preserves query limit, offset and cursor; refuses aliases, parent collections, database/schema qualifiers, filters, joins, grouping, having, ordering, projections and source scan directives. Foreign or typed-nil query/from implementations are refused. |
| `RunReadonlyTransaction` | Returns only a guarded read transaction and bounds its context and lifetime. Retained transaction handles cannot be used after `RunReadonlyTransaction` returns. |

Standing posture `all` permits reads; posture `none` needs a matching, approved,
unexpired connection-level `data.read` grant. Equivalent active connection read
grants form a union: a shorter grant cannot shorten standing read authority or a
longer grant. Agent subjects use the agent posture and cannot inherit human
grants or execute with grants in this checkpoint. Resolutions containing
unsupported object grants or mandatory fine-grained protection fail closed.

All nine typed-write methods remain denied before adapter dispatch: `Set`,
`SetMulti`, `Insert`, `InsertMulti`, `Update`, `UpdateRecord`, `UpdateMulti`,
`Delete` and `DeleteMulti`. Adding a permissive context policy cannot widen the
core's database policy.

## Deadlines and retained values

The initial local deadline uses the interval from verified `ServerTime` to
`ValidUntil`, capped at 60 seconds and, when grant authority is needed, the
applicable grant expiry. The interval starts before resolution I/O, so that
latency consumes the budget. Transactions and readers retain their original
deadline; a refresh cannot extend it.

Point-read staging keeps newly fetched values out of caller-owned aliases when
an adapter returns a partial error or authority expires or is revoked during
I/O. `Next` and `Cursor` check authority before releasing records or tokens and
discard late results on denial. Expiry or parent cancellation closes readers
without waiting for their operation lock, including when an operation is blocked.
Consumers must close readers when finished. Already delivered application
values, caches and snapshots still need their own integration and release checks.

## Unsupported surfaces and claim limits

The returned interface exposes no schema metadata, adapter/native capabilities,
raw handle or read-write transaction opener. Object-grant execution, raw queries,
recordset readers and complex structured queries are unsupported. Full standing
policy/grant composition and production path coverage remain separate work.

The tests prove shared authorization decisions across two recording
implementations, including zero adapter calls for denied typed writes, and the
deadline and value-release guards. They do not certify a native driver, force a
misbehaving adapter to honor cancellation, prove arbitrary database functions
side-effect-free, or prevent direct credential use outside this boundary. Native
read-only modes, credentials and driver assurance require their own evidence.
This foundation alone establishes no CLI, production or agent database guarantee.

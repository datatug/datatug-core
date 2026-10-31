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
`objects.exact`; request admission alone does not enable object-grant execution.
Unknown required features are refused. Plain JSON unmarshalling is not wire
admission.

## Supported execution

| Surface | Current behavior |
| --- | --- |
| `Exists` | Checks authority around the typed read. |
| `Get`, `GetMulti` | Stages data into fresh containers; publishes only after successful read and authority recheck. Supports non-nil maps, non-nil pointers and fixed-length slices. Nil, scalar, key-only or inaccessible targets are refused before dispatch. |
| `ExecuteQueryToRecordsReader` | Accepts only DALgo's built-in, unconditional, single-root-collection key-query profile. Default `SecureDB` accepts only unqualified sources without aliases. The explicit opt-in described below also accepts verified schema/name components and aliases. Both preserve limit, offset and cursor and refuse parent collections, database qualifiers, filters, joins, grouping, having, ordering, projections and source scan directives. Foreign or typed-nil query/from implementations are refused. |
| `RunReadonlyTransaction` | Returns only a guarded read transaction and bounds its context and lifetime. Retained transaction handles cannot be used after `RunReadonlyTransaction` returns. |

Standing posture `all` permits the supported reads. Posture `none` needs an
approved, unexpired `data.read` grant matching the operation. Point reads,
transactions and version1 retained-result capture/import/release require connection-wide
authority; an exact-object grant cannot substitute for it. Active grants form a
union: a shorter grant cannot shorten standing authority or a longer matching
grant, and the existence of an object grant cannot narrow standing `all`. Agent
subjects use the agent posture and cannot inherit human grants or execute with
grants in this checkpoint. Unsupported grant fields and mandatory fine-grained
protection fail closed.

`SecureDBWithTableIdentity` is an optional trusted-host composition point for one
actual opened database. It permits exact-object grants only for the frozen
explicit-schema key-query profile, with database empty and schema/name matching
byte-for-byte. Each operation compiles ordinary DALgo Root/TableScope read rules
from the current posture and active-grant union; DALgo remains the evaluator.
It uses DALgo's shared `ResourcesForQuery` mapping and checks table metadata
before opaque kind, with no raw-query fallback. Bare names never acquire a
namespace from a grant or a prefix. Different literal dots, quotes and case are
different names, and Core never rewrites them.

The optional `TableIdentityVerifier` supplies facts, never permissions. It must
independently corroborate the exact opened handle, authenticated source binding
and native execution identity. It must establish immutable source routing and
identifier/compiler behavior across the complete database/reader lifetime, or
pin an equivalent execution span. Before/after metadata samples alone cannot
prove that a mutable pool did not change and change back. Native folding, quote
interpretation, temporary-schema aliases, default search paths or unsupported
configuration must refuse when byte-exact execution cannot be proved. A flag or
catalog echo alone is insufficient. Logical name equality does not prove
physical relation lineage across DDL replacement.

Core permission candidacy and the original selected deadline precede any fact
lookup. Denied Core authority, unlisted resources and unsupported profiles make
no fact or adapter data calls. Additional restrictive DALgo context policies
intersect before adapter data dispatch, but their denial may follow the trusted
read-only identity-metadata lookup. The fact port receives scalar identity
values and no grants or query payload; it must not mutate data or disclose
secrets. Source/fact failures before or after I/O discard results and close held
readers. Verifiers and resource requirements belong to one secured database and
operation; none are installed globally on Authority.

Missing or nil verifiers do not enable the profile, and default `SecureDB` stays
closed to qualified execution. Synthetic verifier fixtures test composition and
fences only. No production opener or native naming verifier is certified or
wired by this checkpoint.

All nine typed-write methods remain denied before adapter dispatch: `Set`,
`SetMulti`, `Insert`, `InsertMulti`, `Update`, `UpdateRecord`, `UpdateMulti`,
`Delete` and `DeleteMulti`. Adding a permissive context policy cannot widen the
core's database policy.

## Deadlines and retained values

The initial local deadline uses the interval from verified `ServerTime` to
`ValidUntil`, capped at 60 seconds and, when grant authority is needed, the
matching grant expiry. An unrelated surviving object grant cannot extend it.
The interval starts before resolution I/O, so that
latency consumes the budget. Transactions and readers retain their original
deadline; a refresh cannot extend it. Query readers retain exact mapped resource
requirements and grant/protection digest identity, and reauthorize before and
after each page or cursor release. Replacement or downscoped grants cannot
renew an existing reader. Fact-port I/O holds no reader ownership lock, so Close,
cancellation and expiry can close it while a lookup stalls.

Point-read staging keeps newly fetched values out of caller-owned aliases when
an adapter returns a partial error or authority expires or is revoked during
I/O. `Next` and `Cursor` check authority before releasing records or tokens and
discard late results on denial. Expiry or parent cancellation closes readers
without waiting for their operation lock, including when an operation is blocked.
Consumers must close readers when finished. Already delivered application
values, caches and snapshots still need their own integration and release checks.

Result release makes a detached output copy and performs its last local
authorization/deadline check immediately before handoff to the output operation.
That authorized in-flight copy cannot be recalled by later cancellation or
revocation. It stays intact while the writer consumes it and is cleared when
Write returns. Writers must not retain or modify the buffer. Output serialization
has its own mutex: a blocked writer cannot prevent Close, private retained-byte
purge, expiry or native-span cleanup. No ownership mutex spans output I/O.

## Unsupported surfaces and claim limits

`ReadTableResult` is a separate, explicit programmatic table-data profile. Core
constructs one qualified all-column query, secures its native read session with
DALgo's existing operation policy and context-policy intersection, and retains
private serialized cells until a final release check. It admits no caller query,
projection, filter, join, function, raw SQL, view, ordering or pagination. The
bounded preview reads at most N+1 rows and returns at most N, with protected
`returnedRows`, `hasMore` and `complete` metadata. Byte/type/column failures discard
the entire preview. NULL, exact integer tokens and tagged base64 BLOBs are kept
distinct. Mandatory fine-grained restrictions still refuse the profile.

This requires a trusted `TableReadSource` and a pinned `TableReadSpan` that proves
the actual source/table/compiler/native limits before rows open. Later Verify
calls must be SQL-free immutable-proof and liveness checks; fresh control-plane
authority resolution is separate. No old key-profile verifier automatically
enables recordset reads. The span is retained until result close/expiry, and the
data reader must close successfully before any result publication. Final native
cleanup after output cannot retract bytes: the host owns redacted lifecycle-error
reporting. Cleanup and purge never hold an ownership lock across native I/O.

The separate version2 `TableResultEnvelope` binds its exact profile and operation
alongside the original subject/source/request/nonce/digest/deadline. Import
reconstructs resources from the independently expected operation and rejects v1;
v1 import rejects v2. An added or replacement grant cannot rescue a retained
result, and a shorter fresh ceiling is installed before any fact lookup can
stall. No production opener, SQLite native profile or end-to-end consumer route
is enabled merely by this Core API; each needs its own integration proof.

The returned interface exposes no schema metadata, adapter/native capabilities,
raw handle or read-write transaction opener. Its object-only point operations,
transactions, raw queries, general recordset readers and complex structured
queries remain unsupported. Existing version1 result wire behavior is unchanged.
Full standing policy/grant composition, native naming proof and production path
coverage remain separate work.

The tests prove shared authorization decisions across two recording
implementations, including zero adapter calls for denied typed writes, and the
deadline and value-release guards. They do not certify a native driver, force a
misbehaving adapter to honor cancellation, prove arbitrary database functions
side-effect-free, or prevent direct credential use outside this boundary. Native
read-only modes, credentials and driver assurance require their own evidence.
This foundation alone establishes no CLI, production or agent database guarantee.

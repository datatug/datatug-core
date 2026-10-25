# Challenge definition and grading contract

`pkg/challenge` owns the immutable, schema-versioned definition of a guided SQL
challenge and the deterministic primitives that grade a browser-runtime
observation. It does not execute SQL or own attempts, authorization or
completion records.

## Asset and route

A published revision is a project asset at
`challenges/<challengeId>/<revision>.json` (`AssetPath`). The existing query
page opens it in guided mode at the project-scoped route suffix
`challenge/<challengeId>/v/<revision>` (`GuidedRouteSuffix`); the full address
is `/project/<projectRef>/challenge/<id>/v/<revision>`.

Conventional fixture paths are
`challenges/<challengeId>/fixtures/<fixtureId>.db`. Paths are not identity:
each fixture is pinned by SHA-256 and byte length in the manifest.

## Golden wire examples

| Purpose | Path |
| --- | --- |
| Write challenge: SQL final-state oracle + `group-min` invariant + Check-definition part | `testdata/challenges/deduplicate-customers/1.json` |
| Read challenge: typed result oracle + closed multi-select part | `testdata/challenges/retry-evidence/1.json` |
| Pristine-run observations for both challenges | `testdata/goldens/*.json` |
| Language-neutral grading regression vectors | `testdata/grading-vectors.json` |
| Fixture bytes (committed, pinned) | `testdata/fixtures/<challengeId>/*.db` |
| Deterministic generator | `testdata/generate_goldens.py` |

The manifests decode as the Go `Manifest` type and are validated by `Open`.
Unknown fields, duplicate JSON keys, unsupported versions/dialect/engine,
range violations, broken fixture references and closed-option violations are
refused with a stable `challenge.Error` code that reports `Recoverable()`.
`Open` returns no value on refusal, so no attempt or completion can be derived
from a refused definition.

## Grading

- `CompareTypedRows` is the single typed comparison used by both result and
  final-state oracles. It compares a positional column set exactly, ignores
  column aliases unless `requireColumnNames` is set, preserves duplicate
  multiplicity and NULL, and keeps SQLite storage classes distinct per typed
  cell (INTEGER `2` differs from REAL `2.0`) unless the manifest declares a
  non-negative `numericTolerance` (an explicit `"0"` permits numeric equality
  but no difference). Column metadata (`columns[].type`) describes a column and
  is validated for supported shape, but it is never compared with observed
  metadata or with a cell's storage class; heterogeneous results and all-NULL
  columns are therefore legitimate. Expected oracle rows and observed rows use
  the same local shape/value rules.
- Result oracles compare typed rows as an unordered multiset by default; ordered
  comparison happens only when `ordering: "ordered"` is declared.
- Final-state oracles compare every declared table exactly and then evaluate
  named invariants. The observed table set must match the declared set exactly,
  so an undeclared or newly created table cannot pass unnoticed. The only
  invariant in `challenge-grading-v1` is `group-min`: one surviving row per
  declared group whose retention column equals the expected lowest value. The
  retention cell of every expected row must be a non-null numeric value, checked
  by cell value rather than by declared column type; a numeric column declaration
  alone never makes a TEXT cell numeric. This is a challenge-only declarative
  rule, not a production Check accepting arbitrary learner SQL.
- Structured answers use closed field and option IDs and exact set/value
  equality. Unknown fields/options, duplicates and wrong cardinality are
  reported against the part ID without AI judgement.
- Check-definition answers select one declared saved query plus a supported
  typed expectation (`row-count` over the result set). Their outcomes are
  evaluated on the declared broken, repaired and mutated fixtures. Observation
  recordsets pass the same challenge-local column and row shape/value checks,
  so a mixed-result row count is valid without weakening shared
  `pkg/apicontract` validators. No SQL text executes inside this package.
- `GradeAttempt` derives completion only from the manifest's required parts:
  every required part must be `pass` with `completed` execution status.

## Compatibility decisions

- Typed values, recordsets, columns and strict JSON decoding are reused from
  `pkg/apicontract`; Check expectations reuse `detective.Validator`. No parallel
  Check/Work ownership model is introduced.
- `pkg/recordsetcompare` is not reused for grading because it requires
  execution receipts and key-based row identity. Challenge grading is
  receipt-free and positional; forcing it through that API would either fabricate
  receipts or weaken its guarantees. The reusable comparator here is
  `CompareTypedRows`.
- The manifest is public content, so expected rows and correct structured
  selections are visible in the asset. Browser-hidden fixtures are inspectable
  by design and must not be described as secret or tamper-proof.
- The engine is pinned exactly (`SupportedEngine`); a revision graded on another
  engine is refused. Changed data or grading semantics require a new revision.

## Regenerating fixtures and goldens

```sh
cd pkg/challenge
python3 testdata/generate_goldens.py
go test ./pkg/challenge
```

The generator deserializes the committed fixture bytes into a fresh in-memory
database for every answer and every run, so each observation is produced from
pristine bytes. It records the generator's Python/SQLite version in the golden;
the manifests pin the browser engine (`@sqlite.org/sqlite-wasm`
`3.54.1-build1`).

# Training package inventory corpus — version 1

Independent contract data for `walkthrough.AdmitTrainingPackage` (stage 1 of the
training-package programme). Every raw byte and every expected projection or
refusal here is authored by `generate.py` from:

- `sneat-co/backstage` `spec/features/project-walkthroughs/training-package-format.md`
  SHA-256 `6b4d0825e5abad9df4bb4dddb650320076ee49385c4ebaf6aa91f35380446e5a`
- `sneat-co/backstage` `spec/features/project-walkthroughs/declaration-format.md`
  SHA-256 `2411a6074f765d9068fcd6604f9914ac6d80dd1fc008cebbcdc97e2e2f1b8a53`

The generator never imports, runs or reads output from the Go or TypeScript
implementation. The checked-in bytes are the oracle; they are contract test data,
not production story content.

## Layout

```
corpus.json          versioned vectors with raw-file, binding and expected results
provenance.json      SHA-256, byte length and generator/verifier hashes for every committed file
cases/<vector-id>/   raw package index and child files
generate.py          deterministic authoring script (writes the two files above)
verify.py            offline hash/length verification that does not need Go
check_tracked.py     proves every provenance path is a regular blob in the exact Git tree
```

Every case directory contains `training/package.json` (the package index) plus
the declared child files. A case that tests a missing or extra child may omit or
add one. The test harness supplies every file under the case directory except the
index. Ignored `.sqlite` fixtures are committed with an explicit narrow
`git add -f`; `check_tracked.py` fails if any provenance path is untracked or its
committed bytes or length differ.

## Vector schema

`corpus.json` is `{"corpusVersion": "training-package-corpus-v1", "vectors": [...]}`.
Each vector has:

| Field | Meaning |
|---|---|
| `id` | unique vector id, also the case directory name |
| `description` | what the vector proves |
| `directory` | `cases/<id>` |
| `indexPath` | project-relative index path, always `training/package.json` |
| `reference` | `{id, revision, path, sha256, bytes}` pin of the index |
| `binding` | trusted external context `{project, contentPin, declarationSha256, narrativeSha256, packageSha256, scenarioId, scenarioRevision}` |
| `want` | `{admitted, newerClient, definition?, definitionSha256?, issues[]}` |

`definition` is the complete authored `Projection()` for small packages;
`definitionSha256` is the SHA-256 of the compact (sorted-key, no-space) projection
JSON for large boundary packages. `issues` is the exact ordered
`{code, path}` refusal list. A `newerClient` vector has no issues.

Same-source+path aliases may use different codecs. A text alias pair (for
example `query-body-sql` and `evidence-text` over one SQL buffer) and a text plus
`challenge-fixture` pair are both valid: each alias parses independently, and the
unique raw buffer contributes once. The owner keeps the text and binary quotas
separate, so a mixed entry is one physical contribution whose bytes count once
toward each applicable subset; its `class` is `text`, `binary` or `text+binary`.
Declaration order never changes admission or totals. Conflicting pins, invalid
alias bodies and a text alias over 64 KiB still refuse. Every declared source,
used or unused, must resolve a `challenge-fixture` asset.

Unused definitions are admitted only when they are fully closed. Every
`check-definition` asset keeps native Check semantics and package-local
query/Check/catalog closure even when no `checks[]` record binds it, and it never
gains inventory authority. The owner makes `checks[].id` package-unique and ties a
declared definition's native Check id to that record; nothing makes an unused
definition's native id unique, so an unused definition may reuse a declared native
id and is then validated as itself instead of replacing the declared definition in
the dependency graph. A declared `queries[]` record that no Check, proposal or
manifest consumes is still admitted with its closed identity and hydrated native
definition.

A `query-metadata` asset must be bound by a declared `queries[]` record and a
`challenge-manifest` asset by a declared `challenges[]` record, and manifest
`checkDefinition.queryIds` references close against the package `queries[]`
inventory (the reference answer selects one of those same ids natively). Those
reverse bindings are required by the accepted package-closure repair: an
identity-bearing metadata/manifest asset and a manifest saved-query reference are
typed references that must resolve. They are **not yet listed in the owner
document's closure table**, so Apps parity must treat them as pending owner
clarification, not as a settled owner contract.

`Projection()` is the complete conformance projection: every admitted native
Check field (including `title` and the full-precision `updatedAt` encoded the way
the native time value encodes itself) and every native challenge-manifest field,
answer/oracle union, structured field/selection, hint, visible-column option and
reference solution is projected using the owner DTO's `omitempty` normalization.
Absent optional native fields therefore stay absent instead of becoming a
present `false` or empty value. `TrustedPackageInventory()` and
`TrustedNarrativeInventory()` are intentionally reduced identity projections
for later ports; they are not the complete conformance oracle.

## Offline verification (no Go)

```
python3 pkg/walkthrough/testdata/package-corpus-v1/verify.py
python3 pkg/walkthrough/testdata/package-corpus-v1/check_tracked.py [REF]
```

`verify.py` recomputes every raw-file hash and length, the corpus hash, the
generator/verifier hashes, each vector's index pin and the binding package
digest, and fails on any drift. `check_tracked.py` proves each provenance path
is a regular committed blob at `REF` (default `HEAD`) with the exact SHA-256 and
length, and that the tracked corpus tree contains no unexpected files. Neither
script evaluates package semantics, so neither can become a second validator.

## Go conformance

```
cd pkg/walkthrough
go test -run 'TestTrainingPackageCorpus' -count=1
```

Stage 2 consumes these same immutable bytes: copy the case files byte for byte
and assert the hashes above rather than regenerating expectations from the
TypeScript implementation.

## Scope

This corpus covers the package index and all nine owner codecs only. It does not
prove source transport, reservation/reader/disposal ledgers, live authority,
execution, grading, narrative/catalogue-inclusive final text accounting or any
product journey.

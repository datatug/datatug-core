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
provenance.json      SHA-256 of corpus.json and of every committed case file
cases/<vector-id>/   raw package index and child files
generate.py          deterministic authoring script (writes the two files above)
verify.py            offline hash verification that does not need Go
```

Every case directory contains `training/package.json` (the package index) plus
the declared child files. A case that tests a missing or extra child may omit or
add one. The test harness supplies every file under the case directory except the
index.

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

## Offline verification (no Go)

```
python3 pkg/walkthrough/testdata/package-corpus-v1/verify.py
```

This recomputes every raw-file hash, the corpus hash, each vector's index pin and
the binding package digest, and fails on any drift. It does not evaluate package
semantics, so it cannot become a second validator.

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

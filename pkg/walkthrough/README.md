# `pkg/walkthrough` — declaration, pin and checkpoint contracts

Pure Go implementation of the walkthrough contracts owned by
[`spec/features/project-walkthroughs/declaration-format.md`](https://github.com/datatug/backstage/blob/main/spec/features/project-walkthroughs/declaration-format.md)
in `datatug/backstage`:

- the ordinary `datatug-walkthrough/1` declaration;
- the opt-in `datatug-walkthrough/2` training declaration additions;
- immutable asset/package references and source pins;
- the public story registry entry shape and owner-context alias admission;
- the external checkpoint envelope shape, its owner-resolved admission and
  pure match decisions.

It validates bytes supplied by a caller and returns typed projections or
structured recoverable issues. It never reads a project, navigates, publishes
or consumes activity, stores progress, runs SQL or checks, mutates Work or
renders UI. This package is a contract, not the walkthrough runtime.

## `/1` API

```go
result := walkthrough.ParseDeclaration(directoryID, declarationBytes)
if result.NewerClient {
    // the declaration names a schema major this client cannot parse
}
if !result.Accepted {
    // result.Definition is nil; result.Issues is a stable {Code, Path, Message}
}
definition := result.Definition // typed, fully validated, defensively owned
revision := result.Digest       // SHA-256 of the exact input bytes
```

`Declaration.Projection()` is the stable, language-neutral normalized view
shared with the TypeScript contract and the conformance corpus. It exposes
defaults, capture references, support status, entry steps and the digest; it
does not expose presentation prose.

A `/2` declaration passed to `ParseDeclaration` is still reported as
`Result.NewerClient`, exactly as before the `/2` entry point existed.

## `/2` training API

```go
parsed := walkthrough.ParseTrainingDeclaration(directoryID, declarationBytes)
if !parsed.Accepted {
    // parsed.Definition is nil; the /2-only fields were refused
}
declaration := parsed.Definition // structurally valid only

resolution := walkthrough.ResolveTraining(
    declaration,
    walkthrough.SourceContext{Project: identity, ContentPin: pin, DeclarationSHA256: digest},
    projectAuthority, // trusted opaque owner adapter
    trainingOwners,   // trusted validated package/asset/challenge lookup
)
if !resolution.Admitted {
    // structured refusal; no admitted executable bundle is exposed
}
bundle := resolution.Bundle
```

`ParseTrainingDeclaration` is a distinct explicit entry point. It shares the
JSON/variant machinery without widening ordinary `/1` admission: an unknown
target or condition kind stays step-local `needs-newer`, while an unknown step
kind, a wrong variant pairing, a malformed checkpoint mapping or an unsafe pin
refuses the package. A structurally valid parse is never called
"render-ready": package, Case, asset and challenge authority come only from
`ResolveTraining`.

`TrainingDeclaration.Projection()` extends the ordinary normalized view with
the package and narrative references, the step `kind`/`segmentId` and the
checkpoint mapping. `TrainingDeclaration.WalkthroughIdentity()` returns the
declared walkthrough identity used by registry admission.

## Source pins and owner contexts

- `AssetReference` is exactly `path`, `sha256` (64 lowercase hex) and `bytes`
  (positive canonical lexeme). `PackageReference` adds `id` and `revision`.
  `VerifyBytes` checks the exact length and SHA-256 before any parse.
- `ContentPin` is exactly one `{kind: git, commit: <40 hex>}` (normalized
  lowercase) or `{kind: digest, sha256: <64 lowercase hex>}`. A digest pin
  identifies the exact declaration bytes, so it must equal the parsed
  declaration digest at every binding point: `ResolveTraining`,
  registry admission, envelope admission and an active wait refuse a
  shape-valid record whose digest pin and declaration digest disagree.
- `ProjectIdentity` is opaque. This package never interprets it, never builds
  a canonical project serialization and never compares identities itself:
  `ProjectAuthority` supplies `BindProject`/`AdmitProject`/`EqualProject`.
  Production adapters reuse the owning project-context normalization; test
  doubles prove wiring only. Snapshotting the opaque identity value stays the
  owner adapter's responsibility.
- Admission takes an independent snapshot of every contract-owned input. A
  parsed declaration remains caller-owned and mutable, but an admitted bundle
  never aliases it, and the envelope owns a copy of the exact project byte
  span it was parsed from.
- `TrainingOwnerContext` supplies the admitted package inventory, asset
  admission and the exact owner-validated challenge manifest. There is no
  fallback to an ordinary service, no invented package document and no trusted
  "passed" boolean.

## Story registry and checkpoint envelope

`ParseStoryRegistryEntry` reads the closed shape
`storyId/revision/projectUrl/walkthroughId/contentPin/declarationSha256`.
`AdmitStoryChapter` then requires an owner-established canonical immutable
URL, an owner-established public scope, exact pin agreement and a declared
chapter; it returns a `RouteDescriptor` and never invents a walkthrough URL.
Private/unknown scope refuses, and a historical pin mismatch never falls back
to latest.

`ParseCheckpointEnvelope` reads exactly the owner fields, including the opaque
`project` value (copied at parse and handed to the adapter as the exact input
bytes). Inconsistent `outcome`/`complete` summaries refuse.
`AdmitCheckpointEnvelope` resolves the project through the owner adapter and
retains an independent copy of the project span; an inconsistent digest pin
refuses. `MatchCheckpointEnvelope` and `MatchRetainedCompletion` return only a
`consume`/`revisit`/`no-match` decision: consuming once, pause/scheduling,
retained completion storage and authority lifecycle remain runtime owner work.
There is no namespace field on the validation envelope, so a
reference/Test-labelled payload is an unknown-field refusal, never a match.

## Boundaries

- Only `/1` through `ParseDeclaration`, `/2` through
  `ParseTrainingDeclaration`. Later majors are `NewerClient` for both explicit
  entry points; `/1` rejects every `/2`-only field and `/2` refuses a `/1`
  document rather than down-converting it.
- An unknown target or condition `kind` is step-local: the step is kept with
  `SupportNeedsNewer`, its opaque body is discarded rather than exposed or
  executed, and unrelated steps remain usable. Unknown target/condition kinds
  inside a `/2` dialogue/challenge step keep that policy instead of becoming a
  fatal variant-pairing error.
- Captures may be produced only by `parameter-value`, `record-selected` and
  `incident-open`, and never by a `/2` dialogue or challenge step.
- A `finish`/exit `{ "walkthrough": "<id>" }` is checked against the ID grammar.
  Whether that walkthrough exists, and whether any target asset exists or is
  readable, is caller-owned external context.
- Unknown fields reject at every object level. Field names are matched exactly;
  JSON decoding is hand-written because standard Go decoding is case-folding,
  replaces malformed UTF-8 and cannot enforce field presence or nested unions.
- Escaped surrogates use the repository's established Go-compatible treatment:
  valid pairs become one code point, unpaired surrogates become U+FFFD in both
  values and keys. Character budgets count code points after normalization;
  byte budgets count exact input bytes.
- Positive integers keep their canonical decimal lexeme with no
  machine-integer cap, so `2^53`, the `uint64` boundary and larger values
  survive exactly. Fractions, exponents, signs, zero and `null` refuse where a
  positive integer is required.

## Shared locale profile

`AdmitAuthoredLocale` and `NormalizeVisitorLocale` are separate pure entry
points for the `/2` locale contract. Authored fields (sidecar defaults,
catalogue references and catalogue documents) must use values registered in the
pinned IANA snapshots, including the registered private-use ranges; an unknown
but well-formed value is a recoverable `unsupported_registry_version` naming the
snapshot. Visitor requests keep syntactically well-formed unknown values for the
later exact / primary-language / fallback selector, while malformed syntax is a
structured refusal rather than an ambient language.

Both paths enforce the 64-character input and canonical-output bound, ASCII
hyphen-separated grammar, duplicate-variant and duplicate-singleton rejection,
grandfathered/private-use recognition, registered whole-tag and subtag
`Preferred-Value` mappings, contextual extlang replacement, explicit script and
private-tail preservation, and extension-block ordering by singleton. Extension
payloads stay opaque. The returned `Tag` is the canonical comparison key, so
aliases that collide (for example `iw-IL`/`he-IL`) can be rejected as duplicate
catalogue tags. `PrimaryLanguage` is the genuine regular primary language, or
empty for private-use-only and unreplaced irregular grandfathered tags; this
package does not implement the later catalogue selector.

The data lives in `testdata/locale/`: the exact official registry bytes, the
checked-in table generator, `locale-tables.json`, its provenance and the
independently authored expected-value corpus. Tests assert complete runtime
inventory equality with the committed table and every registered grandfathered
and redundant record. Regenerate only after an intentional pinned-snapshot
change:

```sh
python3 pkg/walkthrough/testdata/locale/generate_locale_tables.py
go test ./pkg/walkthrough
```


## Narrative sidecar, catalogues and closure

The pure `/2` narrative slice is split into three modules:

- `narrative.go` — `ParseNarrativeSidecar` (closed schema 1 shape, bounds,
  event registry, payloads, presentation hints and sidecar-local references)
  and `ParseMessageCatalogue` (closed catalogue with stable keys and plain
  text). Both return a typed snapshot or structured recoverable issues.
- `catalogue.go` — `SelectNarrativeLocale` implements exactly the declared
  exact/primary-language/fallback selection using the accepted locale profile;
  `accountNarrativeText` deduplicates canonical source+path aliases, rejects
  conflicting pins, counts different paths separately and enforces the 6 MiB
  combined and 64 KiB per-text-asset budgets.
- `closure.go` — `AdmitNarrative` binds the parsed sidecar to the admitted
  declaration, every pinned byte reference, the package text inventory and the
  trusted `NarrativeOwnerContext`. That extension composes the accepted a2
  port and adds only typed Check, proposal, presentation-target and accounting
  snapshots. Every returned binding carries the exact project/source/
  declaration/package key; a missing, kind-mismatched or drifting binding
  refuses before any admitted executable projection.

A3 is explicitly not a package loader. It neither reads a package index nor
interprets native Check/query/manifest definitions, and it never runs SQL,
Checks or Watches or creates Work. A test double proves closure against its
declared typed inventories only. The caller must construct the context after
the normative package-index/child-byte/codec/native-validation pipeline and
keep it stable for the admission call.

## Corpus

`testdata/corpus.json` is the canonical, language-neutral `/1` conformance
corpus (119 vectors, unchanged). `testdata/corpus-v2.json` is the separately
versioned training corpus; each vector's `operation` selects the explicit
entry point (`training-declaration`, `checkpoint-envelope`,
`story-registry-entry`). Raw bytes live in `testdata/declarations/` and
`testdata/declarations-v2/`. Expected normalized projections are authored from
the owner tables by the independent normalizer in
`testdata/generate_corpus.py`; they are never produced by this package.

`provenance-v2.json` records every file hash, the training corpus hash and the
hash of the unchanged base `/1` corpus. `testdata/corpus-v3.json` is the
separately versioned narrative/catalogue/closure corpus; its `operation` field
selects `narrative-sidecar`, `message-catalogue`, `locale-selection` or
`narrative-admission`. `provenance-v3.json` records every new raw byte file and
the unchanged v2 corpus hash. Raw bytes live under
`testdata/narrative/`. Regenerate fixtures after an intentional corpus change:

```sh
python3 pkg/walkthrough/testdata/generate_corpus.py
python3 pkg/walkthrough/testdata/generate_narrative_corpus.py
go test ./pkg/walkthrough
```

The TypeScript mirror copies these bytes identically and records the Core
commit and per-file SHA-256 in its fixture provenance.

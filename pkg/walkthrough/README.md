# `pkg/walkthrough` — ordinary `/1` declaration contract

Pure Go implementation of the ordinary `datatug-walkthrough/1` contract owned
by [`spec/features/project-walkthroughs/declaration-format.md`](https://github.com/datatug/backstage/blob/main/spec/features/project-walkthroughs/declaration-format.md)
in `datatug/backstage`.

It validates one declaration read from the exact project-relative path
`walkthroughs/<id>/walkthrough.json` and returns either a typed definition or
structured recoverable issues. It never reads a project, navigates, publishes
or consumes activity, stores progress, runs SQL or checks, mutates Work or
renders UI. This package is a contract, not the ordinary `/1` user journey.

## API

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

## Boundaries

- Only `/1` is implemented. Any other schema major is a recoverable
  `Result.NewerClient`; `/2` fields in a `/1` file are fatal unknown fields.
- An unknown target or condition `kind` is step-local: the step is kept with
  `SupportNeedsNewer`, its opaque body is discarded rather than exposed or
  executed, and unrelated steps remain usable. A `/2` target/condition
  discriminator in a `/1` file follows the same rule.
- Captures may be produced only by `parameter-value`, `record-selected` and
  `incident-open`; an unknown condition may still carry a capture under the
  parent-agreed precedence, but exposes no executable producer and emits no
  value at runtime. Later known targets keep their mandatory literal defaults.
- A captured `default` may be `null` only for the `incident` target's
  `incident` value. Literal JSON scalars, including literal `null`, remain
  valid scalars elsewhere.
- A `finish`/exit `{ "walkthrough": "<id>" }` is checked against the ID grammar.
  Whether that walkthrough exists in the same project, and whether any target
  asset exists or is readable, is caller-owned external context and is not read
  here.
- Unknown fields reject at every object level. Field names are matched exactly;
  JSON decoding is hand-written because standard Go decoding is case-folding,
  replaces malformed UTF-8 and cannot enforce field presence or nested unions.
- Escaped surrogates use the repository's established Go-compatible treatment:
  valid pairs become one code point, unpaired surrogates become U+FFFD in both
  values and keys. Character budgets count code points after normalization;
  byte budgets count exact input bytes.

## Corpus

`testdata/corpus.json` is the canonical, language-neutral conformance corpus.
Raw declaration bytes live in `testdata/declarations/`. Expected normalized
projections are authored from the owner tables by the independent normalizer in
`testdata/generate_corpus.py`; they are not produced by this package.

Regenerate fixtures after an intentional corpus change:

```sh
python3 pkg/walkthrough/testdata/generate_corpus.py
go test ./pkg/walkthrough
```

The TypeScript mirror copies these bytes identically and records the Core
commit and per-file SHA-256 in its fixture provenance.

// Package walkthrough owns the pure walkthrough contracts: the ordinary
// `datatug-walkthrough/1` declaration, the opt-in `datatug-walkthrough/2`
// training declaration additions, immutable asset/package references and
// source pins, the public story registry shape and the external checkpoint
// envelope with its pure match decisions.
//
// The package is deliberately passive. It reads bytes supplied by a caller,
// validates them against the normative owner tables in `datatug/backstage`
// `spec/features/project-walkthroughs/declaration-format.md` and returns either
// typed definitions or structured recoverable issues. It never reads a
// project, opens a route, publishes or consumes activity signals, records
// progress, runs SQL or checks, mutates Work, or renders UI. Owner authority
// (project identity, package inventory, Case and challenge admission) enters
// only through the explicit trusted ports in [ProjectAuthority] and
// [TrainingOwnerContext].
//
// # Admission boundaries
//
//   - A declaration is a UTF-8 JSON object of at most [MaxDeclarationBytes].
//   - The declaration ID must equal the directory ID in
//     `walkthroughs/<id>/walkthrough.json` ([DeclarationPath]).
//   - The revision is the SHA-256 of the exact bytes supplied. It is never a
//     hash of a reserialized object.
//   - [ParseDeclaration] admits ordinary /1 only; a /2 document is reported as
//     [Result.NewerClient] exactly as before the explicit /2 entry point
//     existed. [ParseTrainingDeclaration] admits exactly /2 and reports a
//     later major as [TrainingResult.NewerClient].
//   - A structurally valid /2 parse is not a resolved admission. Package,
//     Case, asset, challenge, project and source-pin authority require
//     [ResolveTraining]; a refused resolution never exposes a bundle.
//   - An unknown target or condition `kind` is a step-local newer-client
//     boundary: the step is retained as [SupportNeedsNewer], its opaque body is
//     neither exposed nor executed, and unrelated steps remain usable.
//   - Unknown fields on known variants are fatal. `/1` rejects every
//     /2-only top-level or step field. Unknown step kinds and wrong /2 variant
//     pairings reject the training package.
//   - A refused declaration never yields a partially admitted executable
//     definition.
//
// # Source pins
//
// An asset reference is a safe project-root-relative path, a lowercase 64-hex
// digest and a positive byte count; a package reference adds its ID and
// revision. Supplied bytes are verified against the pin before any parse. A
// content pin is one immutable Git commit (normalized lowercase) or one exact
// content digest. A digest pin identifies the exact declaration bytes, so
// wherever it binds (resolution, registry admission, envelope admission and an
// active wait) it must equal the parsed declaration digest itself; a
// shape-valid record whose two digest fields disagree is never admitted. No
// canonical project serialization is invented here: project identity is an
// opaque value owned by the injected adapter, which also owns its snapshot.
//
// # Defensive snapshots
//
// A parsed declaration, registry entry or envelope draft stays caller-owned and
// mutable. Admission takes an independent deep snapshot of every
// contract-owned input (chapters, steps, target/condition mappings, references,
// pins and the envelope's exact project byte span) and returns projections that
// never alias the caller's structures. Only the opaque project identity value
// itself remains the trusted owner adapter's snapshot responsibility.
//
// # Escaped surrogates
//
// JSON permits unpaired UTF-16 surrogate escapes. Both this package and the
// TypeScript contract deliberately follow the repository's existing
// Go-compatible treatment: a valid high/low pair is one code point, and an
// unpaired surrogate becomes U+FFFD. The same treatment applies to object keys
// and to every string value, so a lone surrogate and a literal U+FFFD compare
// equal in both languages. Character budgets count Unicode code points after
// that normalization; byte budgets always count the exact input bytes.
//
// # Not in this package
//
// Narrative/catalogue JSON schemas, events, presentation and branches,
// segment/chapter/event/reference closure, catalogue selection and
// localisation catalogues, aggregate text accounting, `/2` generation, seek
// and adapter handoffs, navigation, progress storage, activity delivery,
// target existence checks and Walkthrough UI are owned by later slices and
// other packages. The pure locale admission/canonicalization profile in
// [AdmitAuthoredLocale] and [NormalizeVisitorLocale] returns canonical data
// only; the exact/primary/fallback catalogue selector remains later work.
// The ordinary /1 user journey and the training runtime do not exist merely
// because these contracts do.
//
//go:generate python3 testdata/generate_corpus.py
//go:generate python3 testdata/locale/generate_locale_tables.py
package walkthrough

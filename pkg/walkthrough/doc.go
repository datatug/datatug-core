// Package walkthrough owns the pure, closed `datatug-walkthrough/1`
// declaration contract: the declaration bytes, its targets, conditions,
// values and captures, exits, static reference closure and the exact-byte
// revision digest.
//
// The package is deliberately passive. It reads declaration bytes supplied by
// a caller, validates them against the normative owner tables in
// `datatug/backstage` `spec/features/project-walkthroughs/declaration-format.md`
// and returns either a typed definition or structured recoverable issues. It
// never reads a project, opens a route, publishes or consumes activity
// signals, records progress, runs SQL or checks, mutates Work, or renders UI.
//
// # Admission boundaries
//
//   - A declaration is a UTF-8 JSON object of at most [MaxDeclarationBytes].
//   - The declaration ID must equal the directory ID in
//     `walkthroughs/<id>/walkthrough.json` ([DeclarationPath]).
//   - The revision is the SHA-256 of the exact bytes supplied. It is never a
//     hash of a reserialized object.
//   - A declaration whose schema major is newer than /1 is reported as needing
//     a newer DataTug ([Result.NewerClient]) and is not parsed. This package
//     implements /1 only.
//   - An unknown target or condition `kind` is a step-local newer-client
//     boundary: the step is retained as [SupportNeedsNewer], its opaque body is
//     neither exposed nor executed, and unrelated steps remain usable.
//   - Unknown fields on known variants are fatal. `/1` rejects every
//     /2-only top-level or step field. A /2 target/condition discriminator is
//     treated as an unknown kind, not as a supported /1 variant.
//   - A refused declaration never yields a partially admitted executable
//     definition: [Result.Definition] is nil whenever [Result.Accepted] is
//     false.
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
// `datatug-walkthrough/2`, narrative sidecars, source pins, public registry
// aliases, external checkpoint envelopes, localisation, navigation, progress
// storage, activity delivery, target existence checks and Walkthrough UI are
// owned by later slices and other packages. The ordinary /1 user journey does
// not exist merely because this contract does.
//
//go:generate python3 testdata/generate_corpus.py
package walkthrough

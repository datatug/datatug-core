// Package semantic resolves physical columns (a source + collection + set of
// scanned columns) to the entity fields a project's model says they mean.
//
// Resolve is pure - no I/O, no mutation - so it can be called from anywhere
// that already has a project's entities in memory: the datatug-cli HTTP
// resolver behind GET /datatug/semantic/columns (REQ:semantic-resolution-endpoint
// in datatug/datatug's core-investigation-loop feature), the TUI, or a future
// `datatug context` verb. Placement note: the resolver lives here in
// datatug-core rather than in datatug-cli/pkg/semantic (as sketched in the
// Phase-1 plan's task 5) because datatug-cli currently vendors its own copy of
// this package's types and a separate stream is restoring its module
// dependency; the HTTP endpoint itself is still wired in datatug-cli. This is
// a planner decision, not a founder ruling - open to being moved once the
// module dependency lands.
//
// Declared EntityField.Mappings always win over an EntityField.NamePatterns
// match; NamePatterns are tried only for a column with no declared mapping
// anywhere in the project, and the result is labelled Inferred. See
// [Resolve] for the full tie-break rule when more than one field could claim
// the same column.
package semantic

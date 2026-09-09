// Package apicontract holds the Go types for every shared JSON type and
// envelope in the core-investigation-loop Feature's normative transport
// appendix (spec/features/core-investigation-loop/api-contract.md, hub
// datatug/datatug), with the exact json:"..." field names that document
// declares - the schema authority datatug-cli (server) and datatug-apps
// (client) consume, per Phase 1 corrective Task 12.
//
// Every type's Validate method enforces the appendix's own stated rules -
// required fields, closed enums, cross-field pairings, canonical string
// forms - so a value that unmarshals successfully but violates the contract
// is still caught before it crosses a trust boundary. TypedValue and Scope's
// carrying types additionally reject unknown fields, duplicate JSON keys and
// (via DecodeStrict) explicitly named security-relevant fields such as a
// client-supplied principal or role.
//
// pkg/apicontract/fixtures holds the frozen JSON fixtures generated from
// these types - one file per envelope/error case the appendix's "Acceptance
// and migration" section requires coverage for.
package apicontract

// Package challenge owns the immutable, schema-versioned definition of a
// guided SQL challenge and its deterministic grading primitives.
//
// A challenge definition is a project asset (see AssetPath) whose revision
// pins the visible schema, the fixture-suite byte hashes, the answer parts,
// the result/final-state oracles, the grading semantics and the execution
// engine. Opening a definition verifies every pin before any execution is
// allowed; there is no partially trusted manifest.
//
// Grading here is deliberately passive: the browser runtime executes the
// learner's SQL against a disposable fixture and hands the resulting typed
// rows or final table state back to this package. Nothing in this package
// opens a database, executes SQL, performs authorization, calls a model or
// records completion.
//
//go:generate python3 testdata/generate_goldens.py
package challenge

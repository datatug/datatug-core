# Session Context M0 storage spike

This isolated module exercises `sessioncontext.DALRepository` through the mounted
OVDB API and keeps the process-exit M0 reproduction. It pins OVDB Go v0.25.6,
effective DALgo v0.95.1, `dalgo2ingitdb` v0.7.1, ingitdb v0.9.1,
`dalgo2sql` v0.29.0, `dalgo2sqlite` v0.2.3 and modernc SQLite v1.60.1.
The Core replacement points back to this checkout; no machine-specific paths
are used.

Core's requirements make Go's minimal version selection choose DALgo v0.95.2.
This nested module replaces that selection with DALgo v0.95.1 to retain the
`dtql` package required by OVDB Go v0.25.6; DALgo v0.95.2 renamed it to `tugql`.
The replacement applies only to this isolated reproduction. Remove it when
upgrading this fixture to an OVDB Go release that uses `tugql`, and verify the
updated driver graph separately.

Run from this directory:

```sh
go test -count=1 ./...
```

The current full-snapshot API supports at most 64 bags, 400 current entries, and
4 MiB of summed serialized session/bag/context-set/current-entry/current-revision
records. Mutations that grow or replace this read model validate the pending
state before commit and reject over-budget changes; reads fail visibly if stored
data exceeds the bound. This keeps one CLI/dashboard snapshot response bounded.

`TestSQLiteProcessExitAtomicity` is a conformance assertion: reopening after
termination before commit must show the original head, receipt and audit, and
reopening after commit must show all three updates. The SQLite profile is the
required M0 crash-safe baseline.

`TestInGitDBCrashDiagnosticKnownIssue15` is deliberately excluded from the
supported-profile result. It asserts the retained partial-publication outcomes
that reproduce [dalgo2ingitdb issue 15](https://github.com/ingitdb/dalgo2ingitdb/issues/15).
It passing means the known defect still reproduces; it does not mean inGitDB
passes atomicity conformance. If observed outcomes change, the test fails for
review rather than silently converting a diagnostic into a success claim.

`TestFirestoreEmulatorSessionContext` uses the real pinned Firestore adapter to
check transactional commands, reopening, private history pagination and receipt
replay. It skips explicitly unless `FIRESTORE_EMULATOR_HOST` is set. With an owned
local emulator already running, execute:

```sh
FIRESTORE_EMULATOR_HOST=127.0.0.1:8098 go test -run '^TestFirestoreEmulatorSessionContext$' -count=1 -v .
```

This test verifies the isolated module's dependency versions. Hosted HTTP admission,
deployed security rules and the host application's final dependency graph require
their own integration checks.

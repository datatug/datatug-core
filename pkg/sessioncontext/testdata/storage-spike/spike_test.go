package storage_spike

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/sessioncontext"
	"github.com/openvaultdb/openvaultdb-go/pkg/core"
	"github.com/openvaultdb/openvaultdb-go/pkg/mount"
	_ "modernc.org/sqlite"
)

const (
	crashPathEnv  = "SESSION_CONTEXT_M0_PATH"
	crashPointEnv = "SESSION_CONTEXT_M0_POINT"
)

func open(t *testing.T, manifest string) *core.Database {
	t.Helper()
	db, err := mount.File(manifest)
	if err != nil {
		t.Fatalf("mount OVDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func fixture(t *testing.T, engine string) string {
	t.Helper()
	dir := t.TempDir()
	var body string
	switch engine {
	case "sqlite":
		databasePath := filepath.Join(dir, "data.sqlite")
		store, err := sql.Open("sqlite", databasePath)
		if err != nil {
			t.Fatal(err)
		}
		for _, statement := range sessioncontext.SQLiteSchemaStatements() {
			if _, err = store.Exec(statement); err != nil {
				_ = store.Close()
				t.Fatalf("provision session context collection: %v", err)
			}
		}
		if err = store.Close(); err != nil {
			t.Fatal(err)
		}
		body = "database: {id: session-store, schema_mode: strict}\nstorage: {engine: sqlite, path: data.sqlite}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
	case "ingitdb":
		dataPath := filepath.Join(dir, "data")
		if err := os.MkdirAll(dataPath, 0700); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("git", "-C", dataPath, "init").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, output)
		}
		body = "database: {id: session-store, schema_mode: schemaless}\nstorage: {engine: ingitdb, path: data}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
	default:
		t.Fatalf("unsupported engine %q", engine)
	}
	manifest := filepath.Join(dir, "database.yaml")
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestLegacyImportPersistsWithoutDuplicateHistory(t *testing.T) {
	engines := []string{"sqlite"}
	if os.Getenv("FIRESTORE_EMULATOR_HOST") != "" {
		engines = append(engines, "firestore")
	}
	for _, engine := range engines {
		t.Run(engine, func(t *testing.T) {
			manifest := ""
			if engine == "sqlite" {
				manifest = fixture(t, engine)
			} else {
				manifest = filepath.Join(t.TempDir(), "firestore.yaml")
				body := "database: {id: session-store, schema_mode: strict}\nstorage: {engine: firestore, firestore: {project: context-bag-emulator}}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
				if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx := context.Background()
			principal := sessioncontext.Principal{Actor: fmt.Sprintf("legacy-%d", time.Now().UnixNano()), DatabaseID: "session-store", ProjectID: "legacy-project"}
			makeService := func(db *core.Database) *sessioncontext.Service {
				repo, err := sessioncontext.NewDALRepository(db, nil)
				if err != nil {
					t.Fatal(err)
				}
				svc, err := sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
				if err != nil {
					t.Fatal(err)
				}
				return svc
			}
			value := apicontract.NewIntegerValue("9007199254740993")
			historical := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
			command := sessioncontext.Command{OpID: "legacy-import", Action: "importLegacy", DatabaseID: principal.DatabaseID, ProjectID: principal.ProjectID, LegacyImport: &sessioncontext.LegacyImport{
				SourceID: "old-editor", Title: "Imported investigation", Scope: sessioncontext.Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"},
				Entries:  []sessioncontext.LegacyEntry{{ID: "invoice", Subject: sessioncontext.Subject{ConceptID: "Invoice", FieldID: "Invoice.ID", FieldMappingRef: "mapping-r1"}, Value: sessioncontext.ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: &value}, CreatedAt: historical}},
				Messages: []sessioncontext.LegacyChatMessage{{ID: "question", Role: "user", Text: "Historical question", CreatedAt: historical}},
			}}
			db := open(t, manifest)
			first, err := makeService(db).Execute(ctx, principal, command)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			svc := makeService(open(t, manifest))
			command.OpID = "legacy-import-another-operation"
			replayed, err := svc.Execute(ctx, principal, command)
			if err != nil || replayed.Receipt.SessionID != first.Receipt.SessionID || replayed.Receipt.SessionVersion != first.Receipt.SessionVersion {
				t.Fatalf("same source must not create or revise a second session: %v", err)
			}
			snapshot, err := svc.GetSnapshot(ctx, principal, first.Receipt.SessionID)
			if err != nil || len(snapshot.Entries) != 1 || len(snapshot.Revisions) != 1 || snapshot.ContextSet.Enabled || snapshot.Revisions[0].Value.Scalar == nil || snapshot.Revisions[0].Value.Scalar.Str != value.Str {
				t.Fatalf("import must preserve the integer and start unapplied: %v", err)
			}
			messages, err := svc.ListMessages(ctx, principal, first.Receipt.SessionID, "", 10)
			if err != nil || len(messages.Items) != 1 || messages.Items[0].Text != "Historical question" || !messages.Items[0].HistoricalAt.Equal(historical) || messages.Items[0].Provenance != "legacy-import" || messages.Items[0].InputSnapshotID != "" {
				t.Fatalf("historical transcript changed or duplicated: %v", err)
			}
			runs, err := svc.ListRuns(ctx, principal, first.Receipt.SessionID, "", 10)
			if err != nil || len(runs.Items) != 0 {
				t.Fatalf("import fabricated an execution: %v", err)
			}
			command.OpID = "legacy-import-changed-source"
			command.LegacyImport.Messages[0].Text = "Changed source"
			if _, err = svc.Execute(ctx, principal, command); !errors.Is(err, sessioncontext.ErrConflict) {
				t.Fatalf("changed source must fail closed: %v", err)
			}
		})
	}
}

func TestMountedRepositoryReopensSessionContext(t *testing.T) {
	for _, engine := range []string{"sqlite", "ingitdb"} {
		t.Run(engine, func(t *testing.T) {
			manifest := fixture(t, engine)
			db := open(t, manifest)
			repo, err := sessioncontext.NewDALRepository(db, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc, err := sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			principal := sessioncontext.Principal{Actor: "alice", DatabaseID: "session-store", ProjectID: "project1"}
			scope := &sessioncontext.Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
			_, err = svc.Execute(context.Background(), principal, sessioncontext.Command{OpID: "create-session", Action: "createSession", SessionID: "session1", DatabaseID: principal.DatabaseID, ProjectID: principal.ProjectID, BagID: "bag1", Scope: scope, Title: "private"})
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Close(); err != nil {
				t.Fatal(err)
			}
			reopened, err := mount.File(manifest)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			repo, err = sessioncontext.NewDALRepository(reopened, nil)
			if err != nil {
				t.Fatal(err)
			}
			svc, err = sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			page, err := svc.ListSessions(context.Background(), principal, "", 100)
			if err != nil || len(page.Items) != 1 || page.Items[0].ID != "session1" {
				t.Fatalf("reopened session list missing: page=%+v err=%v", page, err)
			}
			snapshot, err := svc.GetSnapshot(context.Background(), principal, "session1")
			if err != nil || snapshot.Session.ID != "session1" || len(snapshot.Bags) != 1 {
				t.Fatalf("reopened session context missing: snapshot=%+v err=%v", snapshot, err)
			}
		})
	}
}

// TestFirestoreEmulatorSessionContext exercises the same MVS as the SQLite
// crash fixture against a real dalgo2firestore driver when the local emulator is
// explicitly enabled. It covers transactional commands, list queries and reopen.
func TestFirestoreEmulatorSessionContext(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set; skipping hosted-storage conformance")
	}
	dir := t.TempDir()
	manifest := filepath.Join(dir, "firestore.yaml")
	content := "database: {id: context-bag-emulator, schema_mode: strict}\nstorage: {engine: firestore, firestore: {project: context-bag-emulator}}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
	if err := os.WriteFile(manifest, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sessionID, bagID, opID := "session-"+suffix, "bag-"+suffix, "create-"+suffix
	scope := &sessioncontext.Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	principal := sessioncontext.Principal{Actor: "alice", DatabaseID: "context-bag-emulator", ProjectID: "p"}
	command := sessioncontext.Command{OpID: opID, Action: "createSession", SessionID: sessionID, DatabaseID: principal.DatabaseID, ProjectID: principal.ProjectID, BagID: bagID, Scope: scope, Title: "emulator session"}
	db := open(t, manifest)
	repo, err := sessioncontext.NewDALRepository(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Execute(context.Background(), principal, command)
	if err != nil {
		t.Fatal(err)
	}
	value := apicontract.NewIntegerValue("9007199254740993")
	put := sessioncontext.Command{OpID: "put-" + suffix, Action: "putEntry", SessionID: sessionID, ExpectedVersion: created.Receipt.SessionVersion, BagID: bagID, ExpectedBagVersion: 1, EntryID: "entry-" + suffix, RevisionID: "revision-" + suffix, Subject: &sessioncontext.Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping-r1"}, Value: &sessioncontext.ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: &value}}
	putResult, err := service.Execute(context.Background(), principal, put)
	if err != nil {
		t.Fatal(err)
	}
	chat := sessioncontext.Command{OpID: "chat-" + suffix, Action: "pinInput", SessionID: sessionID, ExpectedVersion: putResult.Receipt.SessionVersion, InputSnapshotID: "input-" + suffix, RunID: "run-" + suffix, Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "assistant-r1", DefinitionDocument: json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[]}`), Scope: scope, Text: "private question", Attachments: []sessioncontext.EntryRef{{EntryID: put.EntryID, RevisionID: put.RevisionID}}}
	chatResult, err := service.Execute(context.Background(), principal, chat)
	if err != nil {
		t.Fatal(err)
	}
	report := sessioncontext.Command{OpID: "result-" + suffix, Action: "reportClientRun", SessionID: sessionID, ExpectedVersion: chatResult.Receipt.SessionVersion, ExpectedRunVersion: 1, RunID: chat.RunID, Kind: "chat", RunState: "completed", Text: "A large integer."}
	if _, err = service.Execute(context.Background(), principal, report); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := mount.File(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	repo, err = sessioncontext.NewDALRepository(reopened, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err = sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := service.ListSessions(context.Background(), principal, "", 10)
	if err != nil || len(sessions.Items) != 1 || sessions.Items[0].ID != sessionID {
		t.Fatalf("Firestore session list after reopen: page=%+v err=%v", sessions, err)
	}
	history, err := service.ListEntryRevisions(context.Background(), principal, sessionID, put.EntryID, "", 10)
	if err != nil || len(history.Items) != 1 || history.Items[0].ID != put.RevisionID {
		t.Fatalf("Firestore revision history after reopen: page=%+v err=%v", history, err)
	}
	runs, err := service.ListRuns(context.Background(), principal, sessionID, "", 10)
	if err != nil || len(runs.Items) != 1 || runs.Items[0].OutcomeOrigin != "client-reported" {
		t.Fatalf("Firestore run history after reopen: page=%+v err=%v", runs, err)
	}
	messages, err := service.ListMessages(context.Background(), principal, sessionID, "", 10)
	if err != nil || len(messages.Items) != 2 {
		t.Fatalf("Firestore chat history after reopen: page=%+v err=%v", messages, err)
	}
	if _, err = service.Execute(context.Background(), principal, command); err != nil {
		t.Fatalf("Firestore lost-ack replay after reopen: %v", err)
	}
}

// TestProcessExitChild is invoked in a subprocess so os.Exit terminates the
// transaction owner at a deterministic write boundary.
func TestProcessExitChild(t *testing.T) {
	manifest := os.Getenv(crashPathEnv)
	if manifest == "" {
		return
	}
	db := open(t, manifest)
	point := os.Getenv(crashPointEnv)
	err := db.DB().RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		for _, id := range []string{"head", "receipt", "audit"} {
			if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithID("records", id), map[string]any{"version": int64(2)})); err != nil {
				return err
			}
			if id == point {
				os.Exit(93)
			}
		}
		return nil
	}, dal.TxWithMessage("M0 crash fixture"))
	if err != nil {
		t.Fatalf("transaction: %v", err)
	}
	if point == "committed" {
		os.Exit(0)
	}
}

func lowLevelFixture(t *testing.T, engine string) string {
	t.Helper()
	dir := t.TempDir()
	var body string
	if engine == "sqlite" {
		path := filepath.Join(dir, "data.sqlite")
		store, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.Exec(`CREATE TABLE records (id TEXT PRIMARY KEY, version INTEGER)`)
		if err != nil {
			t.Fatal(err)
		}
		_ = store.Close()
		body = "database: {id: m0-fixture, schema_mode: strict}\nstorage: {engine: sqlite, path: data.sqlite}\nschemas:\n  collections:\n    records:\n      fields:\n        version: {type: integer}\n"
	} else {
		data := filepath.Join(dir, "data")
		if err := os.MkdirAll(data, 0700); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("git", "-C", data, "init").CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, output)
		}
		body = "database: {id: m0-fixture, schema_mode: schemaless}\nstorage: {engine: ingitdb, path: data}\nschemas:\n  collections:\n    records:\n      fields:\n        version: {type: integer}\n"
	}
	manifest := filepath.Join(dir, "database.yaml")
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	db := open(t, manifest)
	_, err := db.Apply(context.Background(), []core.Op{{Op: "insert", Key: record.NewKeyWithID("records", "head"), Data: map[string]any{"version": int64(1)}}, {Op: "insert", Key: record.NewKeyWithID("records", "receipt"), Data: map[string]any{"version": int64(1)}}, {Op: "insert", Key: record.NewKeyWithID("records", "audit"), Data: map[string]any{"version": int64(1)}}}, "M0 initial state")
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func childExit(t *testing.T, manifest, point string) error {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestProcessExitChild$")
	command.Env = append(os.Environ(), crashPathEnv+"="+manifest, crashPointEnv+"="+point)
	_, err := command.CombinedOutput()
	return err
}

func versions(t *testing.T, manifest string) [3]int64 {
	t.Helper()
	db, err := mount.File(manifest)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var got [3]int64
	for i, id := range []string{"head", "receipt", "audit"} {
		data, err := db.Get(context.Background(), record.NewKeyWithID("records", id))
		if err != nil {
			t.Fatal(err)
		}
		switch value := data["version"].(type) {
		case int64:
			got[i] = value
		case float64:
			got[i] = int64(value)
		case int:
			got[i] = int64(value)
		default:
			t.Fatalf("unexpected version type %T", value)
		}
	}
	return got
}

func TestSQLiteProcessExitAtomicity(t *testing.T) {
	for _, point := range []string{"head", "receipt", "audit", "committed"} {
		t.Run(point, func(t *testing.T) {
			manifest := lowLevelFixture(t, "sqlite")
			err := childExit(t, manifest, point)
			if point == "committed" {
				if err != nil {
					t.Fatalf("committed child failed: %v", err)
				}
				if got := versions(t, manifest); got != [3]int64{2, 2, 2} {
					t.Fatalf("committed transaction lost writes: %v", got)
				}
				return
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 93 {
				t.Fatalf("child did not exit at %s: %v", point, err)
			}
			if got := versions(t, manifest); got != [3]int64{1, 1, 1} {
				t.Fatalf("SQLite transaction exposed partial %s writes after reopen: %v", point, got)
			}
		})
	}
}

func TestInGitDBCrashDiagnosticKnownIssue15(t *testing.T) {
	want := map[string][3]int64{
		"head":      {2, 1, 1},
		"receipt":   {2, 2, 1},
		"audit":     {2, 2, 2},
		"committed": {2, 2, 2},
	}
	for point, expected := range want {
		t.Run(point, func(t *testing.T) {
			manifest := lowLevelFixture(t, "ingitdb")
			err := childExit(t, manifest, point)
			if point == "committed" {
				if err != nil {
					t.Fatalf("committed child failed: %v", err)
				}
			} else {
				var exit *exec.ExitError
				if !errors.As(err, &exit) || exit.ExitCode() != 93 {
					t.Fatalf("child did not exit at %s: %v", point, err)
				}
			}
			if got := versions(t, manifest); got != expected {
				t.Fatalf("issue 15 diagnostic changed at %s: got %v, want %v", point, got, expected)
			}
		})
	}
}

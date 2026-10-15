package storage_spike

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/sessioncontext"
	"github.com/openvaultdb/openvaultdb-go/pkg/core"
)

func nativePinService(t *testing.T, db *core.Database, p sessioncontext.Principal) (*sessioncontext.Service, *sessioncontext.DALRepository) {
	t.Helper()
	repo, err := sessioncontext.NewNativePinRepository(db, nil, "local", "", p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sessioncontext.NewNativePinService(repo, func(context.Context, sessioncontext.Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return s, repo
}
func nativePin(t *testing.T, s *sessioncontext.Service, repo *sessioncontext.DALRepository, p sessioncontext.Principal) sessioncontext.PinDetectiveInputCommand {
	t.Helper()
	ctx := context.Background()
	scope := sessioncontext.Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	created, err := s.Execute(ctx, p, sessioncontext.Command{OpID: "create", Action: "createSession", SessionID: p.Actor, BagID: p.Actor + "-bag", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Scope: &scope})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.GetSnapshot(ctx, p, p.Actor)
	if err != nil {
		t.Fatal(err)
	}
	definition := sessioncontext.DetectiveCheckDefinitionPin{SchemaVersion: 1, Check: detective.Check{ID: "check", Title: "Private Check", Version: 1, Steps: []detective.Step{{ID: "q", Kind: "query", QueryID: "q"}}}, Queries: []sessioncontext.DetectiveAcceptedQueryPin{{QueryID: "q", Revision: "r1", DefinitionJSON: json.RawMessage(`{"id":"q","title":"Private query","type":"SQL","text":"select private_value"}`)}}}
	_, digest, err := sessioncontext.CanonicalDetectiveDefinition(ctx, definition)
	if err != nil {
		t.Fatal(err)
	}
	n := repo.NativeNamespace()
	ref := apicontract.SessionInputReference{SchemaVersion: 1, StorageID: n.StorageID, SpaceID: n.SpaceID, DatabaseID: n.DatabaseID, ProjectID: n.ProjectID, NamespaceID: n.NamespaceID, SessionID: p.Actor, InputSnapshotID: p.Actor + "-input", ContextSetID: snap.ContextSet.ID, EnvironmentID: scope.EnvironmentID, IdentityDomain: scope.IdentityDomain, DefinitionDigest: digest}
	inputs := map[string]detective.InputValue{}
	payload, err := detective.ActionOperationPayload(detective.ActionRequest{Type: "run-check", Project: p.ProjectID, CheckID: "check", Environment: scope.EnvironmentID, Inputs: inputs, InputReference: &ref, ExpectedDefinitionDigest: digest})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := detective.BindOperation(p.ProjectID, p.Actor, "run-check", "check", payload)
	if err != nil {
		t.Fatal(err)
	}
	return sessioncontext.PinDetectiveInputCommand{OpID: "pin", SessionID: p.Actor, InputSnapshotID: ref.InputSnapshotID, RunID: p.Actor + "-run", ExpectedVersion: created.Receipt.SessionVersion, ExpectedContextSetID: ref.ContextSetID, Scope: scope, NativeBinding: binding, Pin: sessioncontext.DetectiveInputPin{Reference: ref, Definition: definition, EffectiveInputs: inputs, Dispatch: sessioncontext.DetectiveDispatchBinding{NativeOperationID: p.Actor + "-native", NativeStoreID: "local", NativeProjectID: p.ProjectID, Action: "run-check", CheckID: "check", CheckVersion: 1, DefinitionDigest: digest, NativeRequestDigest: binding.PayloadDigest}}}
}

func TestSQLiteNativePinAtomicFailureAndReopen(t *testing.T) {
	manifest := fixture(t, "sqlite")
	db := open(t, manifest)
	p := sessioncontext.Principal{Actor: "native-owner", DatabaseID: "session-store", ProjectID: "project", Executor: true}
	s, repo := nativePinService(t, db, p)
	c := nativePin(t, s, repo, p)
	raw, err := sql.Open("sqlite", filepath.Join(filepath.Dir(manifest), "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_native_work BEFORE INSERT ON "sessionContextDetectiveWork" BEGIN SELECT RAISE(ABORT, 'injected write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PinDetectiveInput(context.Background(), p, c); err == nil {
		t.Fatal("injected adapter write accepted")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, repo = nativePinService(t, open(t, manifest), p)
	err = repo.View(context.Background(), func(r sessioncontext.Reader) error {
		for _, key := range []struct{ collection, id string }{{sessioncontext.Inputs, c.InputSnapshotID}, {sessioncontext.Runs, c.RunID}, {sessioncontext.DetectiveWork, c.RunID}} {
			var value any
			if _, e := r.Load(context.Background(), key.collection, key.id, &value); !errors.Is(e, sessioncontext.ErrNotFound) {
				t.Fatalf("partial adapter record %s: %v", key.collection, e)
			}
		}
		for _, collection := range []string{sessioncontext.Operations, sessioncontext.Audit} {
			rows, _, err := r.List(context.Background(), collection, p.Actor, "", 10)
			if err != nil || len(rows) != 1 {
				t.Fatalf("partial adapter receipt/audit %s: count=%d err=%v", collection, len(rows), err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := s.GetSnapshot(context.Background(), p, p.Actor)
	if err != nil || snap.Session.PendingRunCount != 0 || snap.Session.Version != 1 {
		t.Fatalf("adapter rollback lost session: %v", err)
	}
	if _, err = raw.Exec(`DROP TRIGGER fail_native_work`); err != nil {
		t.Fatal(err)
	}
	first, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil || first.Receipt.PayloadDigest != second.Receipt.PayloadDigest || first.Receipt.SessionVersion != second.Receipt.SessionVersion {
		t.Fatalf("adapter replay: %v", err)
	}
	if _, err = s.ReadDetectiveDispatch(context.Background(), p, p.Actor, c.RunID); err != nil {
		t.Fatal(err)
	}
}

func TestFirestoreEmulatorNativePinReplayAndMarker(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("real Firestore adapter requires FIRESTORE_EMULATOR_HOST")
	}
	manifest := filepath.Join(t.TempDir(), "firestore.yaml")
	body := "database: {id: session-store, schema_mode: strict}\nstorage: {engine: firestore, firestore: {project: context-bag-emulator}}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	p := sessioncontext.Principal{Actor: fmt.Sprintf("native-%d", time.Now().UnixNano()), DatabaseID: "session-store", ProjectID: "project", Executor: true}
	db := open(t, manifest)
	s, repo := nativePinService(t, db, p)
	c := nativePin(t, s, repo, p)
	first, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	s, repo = nativePinService(t, open(t, manifest), p)
	again, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil || first.Receipt.PayloadDigest != again.Receipt.PayloadDigest {
		t.Fatalf("Firestore replay: %v", err)
	}
	if err = repo.Transact(context.Background(), func(tx sessioncontext.Transaction) error {
		return tx.Save(context.Background(), sessioncontext.Redactions, p.Actor, sessioncontext.SessionRedaction{SessionID: p.Actor, Owner: p.Actor, Namespace: repo.NativeNamespace(), Version: 1}, 0, p.Actor, p.Actor)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ReadDetectiveDispatch(context.Background(), p, p.Actor, c.RunID); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatalf("Firestore marker bypass: %v", err)
	}
	if _, err = s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatalf("Firestore stale pin bypass: %v", err)
	}
}

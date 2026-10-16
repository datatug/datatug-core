package storage_spike

import (
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/sessioncontext"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSQLiteSnapshotWireRetainsAppliedRevisionAcrossReopen(t *testing.T) {
	ctx := context.Background()
	manifest := fixture(t, "sqlite")
	db := open(t, manifest)
	makeService := func() *sessioncontext.Service {
		t.Helper()
		repo, e := sessioncontext.NewDALRepository(db, nil)
		if e != nil {
			t.Fatal(e)
		}
		svc, e := sessioncontext.NewService(repo, func(context.Context, sessioncontext.Access) error { return nil })
		if e != nil {
			t.Fatal(e)
		}
		return svc
	}
	svc := makeService()
	p := sessioncontext.Principal{Actor: "alice", DatabaseID: "session-store", ProjectID: "p"}
	scope := sessioncontext.Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	execute := func(c sessioncontext.Command) sessioncontext.Receipt {
		t.Helper()
		r, e := svc.Execute(ctx, p, c)
		if e != nil {
			t.Fatalf("%s: %v", c.Action, e)
		}
		return r.Receipt
	}
	created := execute(sessioncontext.Command{OpID: "create", Action: "createSession", SessionID: "s", BagID: "b", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Scope: &scope, Title: "Invoice investigation"})
	value := apicontract.NewIntegerValue("1")
	put := sessioncontext.Command{OpID: "put-a", Action: "putEntry", SessionID: "s", BagID: "b", ExpectedVersion: created.SessionVersion, ExpectedBagVersion: 1, EntryID: "invoice", RevisionID: "a", Subject: &sessioncontext.Subject{ConceptID: "Invoice", FieldID: "InvoiceId", FieldMappingRef: "mapping"}, Value: &sessioncontext.ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: &value}}
	saved := execute(put)
	snap, e := svc.GetSnapshot(ctx, p, "s")
	if e != nil {
		t.Fatal(e)
	}
	apply := sessioncontext.Command{OpID: "apply-a", Action: "applyContext", SessionID: "s", BagID: "b", ExpectedVersion: saved.SessionVersion, ExpectedBagVersion: 2, ExpectedContextSetID: snap.ContextSet.ID, Scope: &scope, Enabled: new(true), Selection: []sessioncontext.EntryRef{{EntryID: "invoice", RevisionID: "a"}}}
	applied := execute(apply)
	pinned := execute(sessioncontext.Command{OpID: "pin", Action: "pinInput", SessionID: "s", ExpectedVersion: applied.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[]}`), Scope: &scope, Attachments: apply.Selection})
	value = apicontract.NewIntegerValue("2")
	put.OpID = "put-b"
	put.RevisionID = "b"
	put.ExpectedVersion = pinned.SessionVersion
	put.ExpectedBagVersion = 2
	put.ExpectedEntryVersion = 1
	edited := execute(put)
	// Reopen real SQLite, then exercise HTTP JSON serialization without a listener.
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	db = open(t, manifest)
	svc = makeService()
	recorder := httptest.NewRecorder()
	http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		snapshot, err := svc.GetSnapshot(ctx, p, "s")
		if err != nil {
			t.Error(err)
			w.WriteHeader(500)
			return
		}
		if err = json.NewEncoder(w).Encode(snapshot); err != nil {
			t.Error(err)
		}
	}).ServeHTTP(recorder, httptest.NewRequest("GET", "/snapshot", nil))
	if recorder.Code != 200 {
		t.Fatalf("wire status %d", recorder.Code)
	}
	var wire sessioncontext.Snapshot
	if e = json.Unmarshal(recorder.Body.Bytes(), &wire); e != nil {
		t.Fatal(e)
	}
	if len(wire.Revisions) != 2 || wire.ContextSet.Slots[0].Entries[0].RevisionID != "a" || wire.Entries[0].CurrentRevisionID != "b" {
		t.Fatalf("wire union: %+v", wire)
	}
	values := map[string]string{}
	for _, r := range wire.Revisions {
		values[r.ID] = r.Value.Scalar.Str
	}
	if values["a"] != "1" || values["b"] != "2" {
		t.Fatalf("wire values: %v", values)
	}
	apply.OpID = "apply-b"
	apply.ExpectedVersion = edited.SessionVersion
	apply.ExpectedBagVersion = 3
	apply.ExpectedSlotVersion = 1
	apply.ExpectedContextSetID = wire.ContextSet.ID
	apply.Selection = []sessioncontext.EntryRef{{EntryID: "invoice", RevisionID: "b"}}
	reapplied := execute(apply)
	wire, e = svc.GetSnapshot(ctx, p, "s")
	if e != nil || len(wire.Revisions) != 1 || wire.Revisions[0].ID != "b" {
		t.Fatalf("reapply: %+v %v", wire, e)
	}
	execute(sessioncontext.Command{OpID: "clear", Action: "clearBag", SessionID: "s", BagID: "b", ExpectedVersion: reapplied.SessionVersion, ExpectedBagVersion: 3, ExpectedSlotVersion: 2, Scope: &scope})
	wire, e = svc.GetSnapshot(ctx, p, "s")
	if e != nil || len(wire.Revisions) != 0 {
		t.Fatalf("clear: %+v %v", wire, e)
	}
	_, revisions, e := svc.ReadInput(ctx, p, "input")
	if e != nil || len(revisions) != 1 || revisions[0].Value.Scalar.Str != "1" {
		t.Fatalf("retained input: %+v %v", revisions, e)
	}
}

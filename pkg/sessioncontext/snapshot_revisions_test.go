package sessioncontext

import (
	"context"
	"errors"
	"fmt"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotRetainsAppliedRevisionUntilReapply(t *testing.T) {
	svc, p := newTestService(t, nil)
	created := createTestSession(t, svc, p, "history-session", "history-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	subject := Subject{ConceptID: "Invoice", FieldID: "InvoiceId", FieldMappingRef: "invoice-mapping"}
	put := Command{OpID: "put-a", Action: "putEntry", SessionID: "history-session", ExpectedVersion: created.SessionVersion, BagID: "history-bag", ExpectedBagVersion: 1, EntryID: "invoice", RevisionID: "revision-a", Scope: &scope, Subject: &subject, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}}
	saved := executeOK(t, svc, p, put)
	snapshot, err := svc.GetSnapshot(context.Background(), p, put.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	apply := Command{OpID: "apply-a", Action: "applyContext", SessionID: put.SessionID, ExpectedVersion: saved.SessionVersion, BagID: put.BagID, ExpectedBagVersion: 2, ExpectedContextSetID: snapshot.ContextSet.ID, Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: put.EntryID, RevisionID: put.RevisionID}}}
	applied := executeOK(t, svc, p, apply)
	pin := Command{OpID: "pin-a", Action: "pinInput", SessionID: put.SessionID, ExpectedVersion: applied.SessionVersion, InputSnapshotID: "input-a", RunID: "run-a", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "assistant-r1", DefinitionDocument: testDefinition("chat"), Scope: &scope, Attachments: []EntryRef{{EntryID: put.EntryID, RevisionID: "revision-a"}}}
	pinned := executeOK(t, svc, p, pin)
	put.OpID = "put-b"
	put.RevisionID = "revision-b"
	put.ExpectedVersion = pinned.SessionVersion
	put.ExpectedBagVersion = 2
	put.ExpectedEntryVersion = 1
	put.Value = &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("2"))}
	edited := executeOK(t, svc, p, put)
	snapshot, err = svc.GetSnapshot(context.Background(), p, put.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Revisions) != 2 || snapshot.ContextSet.Slots[0].Entries[0].RevisionID != "revision-a" {
		t.Fatalf("applied A and current B must both resolve: %+v", snapshot)
	}
	values := map[string]string{}
	for _, r := range snapshot.Revisions {
		values[r.ID] = r.Value.Scalar.Str
	}
	if values["revision-a"] != "1" || values["revision-b"] != "2" {
		t.Fatalf("immutable values lost: %v", values)
	}
	apply.OpID = "apply-b"
	apply.ExpectedVersion = edited.SessionVersion
	apply.ExpectedBagVersion = 3
	apply.ExpectedSlotVersion = 1
	apply.ExpectedContextSetID = snapshot.ContextSet.ID
	apply.Selection = []EntryRef{{EntryID: put.EntryID, RevisionID: put.RevisionID}}
	reapplied := executeOK(t, svc, p, apply)
	snapshot, err = svc.GetSnapshot(context.Background(), p, put.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Revisions) != 1 || snapshot.Revisions[0].ID != "revision-b" {
		t.Fatalf("reapply must prune A from current snapshot: %+v", snapshot.Revisions)
	}
	executeOK(t, svc, p, Command{OpID: "clear-history", Action: "clearBag", SessionID: put.SessionID, ExpectedVersion: reapplied.SessionVersion, BagID: put.BagID, ExpectedBagVersion: 3, ExpectedSlotVersion: 2, Scope: &scope})
	snapshot, err = svc.GetSnapshot(context.Background(), p, put.SessionID)
	if err != nil || len(snapshot.Revisions) != 0 {
		t.Fatalf("clear snapshot: %+v %v", snapshot, err)
	}
	_, retained, err := svc.ReadInput(context.Background(), p, "input-a")
	if err != nil || len(retained) != 1 || retained[0].Value.Scalar.Str != "1" {
		t.Fatalf("retained input A lost: %+v %v", retained, err)
	}
}

func TestSnapshotAppliedReferencesFailClosed(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Session, *Bag, *AppliedContextSet, *Entry, *EntryRevision, *EntryRevision)
	}{
		{"context set identity", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.ID = "foreign"
		}},
		{"context set session", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.SessionID = "foreign"
		}},
		{"foreign bag", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].BagID = "foreign"
		}},
		{"wrong domain", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].Scope.IdentityDomain = "foreign"
		}},
		{"future bag version", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].BagVersion = 3
		}},
		{"before clear", func(_ *Session, b *Bag, _ *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			b.ClearVersion = 2
		}},
		{"duplicate scope", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots = append(s.Slots, s.Slots[0])
		}},
		{"duplicate entry", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].Entries = append(s.Slots[0].Entries, s.Slots[0].Entries[0])
		}},
		{"invalid revision ID", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].Entries[0].RevisionID = "../private"
		}},
		{"missing historical revision", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].Entries[0].RevisionID = "missing"
		}},
		{"historical identity", func(_ *Session, _ *Bag, _ *AppliedContextSet, _ *Entry, a *EntryRevision, _ *EntryRevision) {
			a.ID = "foreign"
		}},
		{"historical entry", func(_ *Session, _ *Bag, _ *AppliedContextSet, _ *Entry, a *EntryRevision, _ *EntryRevision) {
			a.EntryID = "foreign"
		}},
		{"historical bag", func(_ *Session, _ *Bag, _ *AppliedContextSet, _ *Entry, a *EntryRevision, _ *EntryRevision) {
			a.BagID = "foreign"
		}},
		{"current identity", func(_ *Session, _ *Bag, _ *AppliedContextSet, _ *Entry, _ *EntryRevision, b *EntryRevision) {
			b.ID = "foreign"
		}},
		{"entry identity", func(_ *Session, _ *Bag, _ *AppliedContextSet, e *Entry, _ *EntryRevision, _ *EntryRevision) {
			e.ID = "foreign"
		}},
		{"entry session", func(_ *Session, _ *Bag, _ *AppliedContextSet, e *Entry, _ *EntryRevision, _ *EntryRevision) {
			e.SessionID = "foreign"
		}},
		{"tombstone refs", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].BagID = ""
			s.Slots[0].BagVersion = 0
		}},
		{"too many slots", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots = make([]AppliedSlot, 65)
		}},
		{"too many refs", func(_ *Session, _ *Bag, s *AppliedContextSet, _ *Entry, _ *EntryRevision, _ *EntryRevision) {
			s.Slots[0].Entries = make([]EntryRef, MaxSnapshotEntries+1)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, p := newTestService(t, nil)
			v := Session{ID: "session", ContextSetID: "set", BagIDs: []string{"bag"}}
			b := Bag{ID: "bag", SessionID: v.ID, Version: 2, Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, EntryIDs: []string{"entry"}}
			e := Entry{ID: "entry", SessionID: v.ID, BagID: b.ID, CurrentRevisionID: "b"}
			a := EntryRevision{ID: "a", EntryID: e.ID, BagID: b.ID}
			head := a
			head.ID = "b"
			set := AppliedContextSet{ID: "set", SessionID: v.ID, Slots: []AppliedSlot{{Scope: b.Scope, BagID: b.ID, BagVersion: 1, Version: 1, Entries: []EntryRef{{EntryID: e.ID, RevisionID: "a"}}}}}
			tc.mutate(&v, &b, &set, &e, &a, &head)
			r := singleRecordReader{records: map[string]any{ContextSets + "/set": set, Bags + "/bag": b, Entries + "/entry": e, Revisions + "/a": a, Revisions + "/b": head}, versions: map[string]int64{Bags + "/bag": 2}}
			snapshot, err := svc.loadSnapshot(context.Background(), r, p, v, "read")
			if err == nil || len(snapshot.Revisions) != 0 {
				t.Fatalf("malformed reference exposed snapshot: %+v err=%v", snapshot, err)
			}
		})
	}
}

func TestSnapshotHistoricalBytesRollbackEdit(t *testing.T) {
	svc, p := newTestService(t, nil)
	ctx := context.Background()
	created := createTestSession(t, svc, p, "byte-union", "byte-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	refs := []EntryRef{}
	version := created.SessionVersion
	bagVersion := int64(1)
	for i := 0; i < 6; i++ {
		id := fmt.Sprintf("entry-%d", i)
		rev := fmt.Sprintf("revision-%d", i)
		c := Command{OpID: "put-" + id, Action: "putEntry", SessionID: "byte-union", ExpectedVersion: version, BagID: "byte-bag", ExpectedBagVersion: bagVersion, EntryID: id, RevisionID: rev, Subject: &Subject{ConceptID: "Note", FieldID: "Text", FieldMappingRef: "text-mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue(strings.Repeat("x", 690000)))}}
		saved := executeOK(t, svc, p, c)
		version = saved.SessionVersion
		bagVersion++
		refs = append(refs, EntryRef{EntryID: id, RevisionID: rev})
	}
	before, err := svc.GetSnapshot(ctx, p, "byte-union")
	if err != nil {
		t.Fatal(err)
	}
	applied := executeOK(t, svc, p, Command{OpID: "apply-large", Action: "applyContext", SessionID: "byte-union", ExpectedVersion: version, BagID: "byte-bag", ExpectedBagVersion: bagVersion, Scope: &scope, ExpectedContextSetID: before.ContextSet.ID, Enabled: boolPtr(true), Selection: refs})
	before, err = svc.GetSnapshot(ctx, p, "byte-union")
	if err != nil {
		t.Fatal(err)
	}
	edit := Command{OpID: "over-budget-edit", Action: "putEntry", SessionID: "byte-union", ExpectedVersion: applied.SessionVersion, BagID: "byte-bag", ExpectedBagVersion: bagVersion, EntryID: "entry-0", ExpectedEntryVersion: 1, RevisionID: "new-revision", Subject: &Subject{ConceptID: "Note", FieldID: "Text", FieldMappingRef: "text-mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue(strings.Repeat("y", 100000)))}}
	if _, err = svc.Execute(ctx, p, edit); !errors.Is(err, ErrInvalid) {
		t.Fatalf("historical union overflow accepted: %v", err)
	}
	after, err := svc.GetSnapshot(ctx, p, "byte-union")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("overflow changed readable state")
	}
	if err = svc.repo.View(ctx, func(r Reader) error {
		var event AuditEvent
		_, err := r.Load(ctx, Audit, opKey(p, edit.OpID), &event)
		return err
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("overflow wrote audit event: %v", err)
	}
	if _, err = svc.ReadOperation(ctx, p, edit.OpID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("overflow wrote receipt: %v", err)
	}
	if err = svc.repo.View(ctx, func(r Reader) error {
		var rev EntryRevision
		_, err := r.Load(ctx, Revisions, edit.RevisionID, &rev)
		return err
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("overflow retained revision: %v", err)
	}
}

func TestSnapshotRevisionUnionBoundAndDedup(t *testing.T) {
	svc, p := newTestService(t, nil)
	v := Session{ID: "union", ContextSetID: "set", BagIDs: []string{"bag"}}
	b := Bag{ID: "bag", SessionID: v.ID, Version: 2, Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}
	set := AppliedContextSet{ID: "set", SessionID: v.ID, Enabled: false, Slots: []AppliedSlot{{Scope: b.Scope, BagID: b.ID, BagVersion: 1, Version: 1}}}
	r := singleRecordReader{records: map[string]any{}, versions: map[string]int64{Bags + "/bag": 2}}
	for i := 0; i < MaxSnapshotEntries; i++ {
		eid := fmt.Sprintf("entry-%d", i)
		head := fmt.Sprintf("head-%d", i)
		old := fmt.Sprintf("old-%d", i)
		b.EntryIDs = append(b.EntryIDs, eid)
		set.Slots[0].Entries = append(set.Slots[0].Entries, EntryRef{EntryID: eid, RevisionID: old})
		r.records[Entries+"/"+eid] = Entry{ID: eid, SessionID: v.ID, BagID: b.ID, CurrentRevisionID: head, Subject: Subject{FieldID: "new-field"}}
		r.records[Revisions+"/"+head] = EntryRevision{ID: head, EntryID: eid, BagID: b.ID}
		r.records[Revisions+"/"+old] = EntryRevision{ID: old, EntryID: eid, BagID: b.ID, Subject: Subject{FieldID: "historical-field"}}
		// Unreferenced intermediate history must never be loaded.
		r.records[Revisions+"/unused-"+eid] = EntryRevision{ID: "unused-" + eid, EntryID: eid, BagID: b.ID}
	}
	r.records[Bags+"/bag"] = b
	r.records[ContextSets+"/set"] = set
	snapshot, err := svc.loadSnapshot(context.Background(), r, p, v, "read")
	if err != nil || len(snapshot.Revisions) != MaxSnapshotRevisions {
		t.Fatalf("disabled-master full union: %d %v", len(snapshot.Revisions), err)
	}
	for i := range set.Slots[0].Entries {
		set.Slots[0].Entries[i].RevisionID = fmt.Sprintf("head-%d", i)
	}
	r.records[ContextSets+"/set"] = set
	snapshot, err = svc.loadSnapshot(context.Background(), r, p, v, "read")
	if err != nil || len(snapshot.Revisions) != MaxSnapshotEntries {
		t.Fatalf("current/applied dedup: %d %v", len(snapshot.Revisions), err)
	}
}

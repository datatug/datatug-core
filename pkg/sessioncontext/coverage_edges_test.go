package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func TestReadMethodsAndSnapshotRejectStorageCorruption(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "edge-session", "edge-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "edge-put", Action: "putEntry", SessionID: "edge-session", ExpectedVersion: created.SessionVersion, BagID: "edge-bag", ExpectedBagVersion: 1, EntryID: "edge-entry", RevisionID: "edge-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m1"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	apply := Command{OpID: "edge-apply", Action: "applyContext", SessionID: "edge-session", ExpectedVersion: putReceipt.SessionVersion, BagID: "edge-bag", ExpectedBagVersion: 2, ExpectedContextSetID: "cs_" + opKey(p, "create-edge-session"), Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: "edge-entry", RevisionID: "edge-revision"}}}
	applyReceipt := executeOK(t, base, p, apply)
	pin := Command{OpID: "edge-pin", Action: "pinInput", SessionID: "edge-session", ExpectedVersion: applyReceipt.SessionVersion, InputSnapshotID: "edge-input", RunID: "edge-run", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope}
	executeOK(t, base, p, pin)

	if _, err := base.GetSnapshot(context.Background(), Principal{Actor: "bob", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID}, "edge-session"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("snapshot owner check: %v", err)
	}
	if _, err := base.ListEntryRevisions(context.Background(), p, "edge-session", "unsafe id", "", 10); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revision ID validation: %v", err)
	}
	if _, err := base.ReadEntryRevision(context.Background(), p, "edge-session", "edge-entry", "unsafe id"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("revision read ID validation: %v", err)
	}
	if err := base.repo.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Revisions, "other-revision", EntryRevision{ID: "other-revision", EntryID: "another-entry", BagID: "edge-bag"}, 0, p.Actor, "edge-session")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadEntryRevision(context.Background(), p, "edge-session", "edge-entry", "other-revision"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revision ownership check: %v", err)
	}
	if _, _, err := base.ReadInput(context.Background(), p, "missing-input"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing input: %v", err)
	}
	if _, err := base.ReadRun(context.Background(), p, "missing-run"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing run: %v", err)
	}

	faults := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == Revisions && id == "edge-revision" {
			return ErrUnavailable
		}
		return nil
	}}
	broken := interceptService(t, base, p, faults)
	if _, _, err := broken.ReadInput(context.Background(), p, "edge-input"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revision load failure: %v", err)
	}
	if _, err := broken.GetSnapshot(context.Background(), p, "edge-session"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot revision load failure: %v", err)
	}

	badSet := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == ContextSets {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, base, p, badSet).GetSnapshot(context.Background(), p, "edge-session"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot context-set load failure: %v", err)
	}
}

func TestValueAndDefinitionValidatorsRejectMalformedNestedValues(t *testing.T) {
	for name, value := range map[string]ValueEnvelope{
		"schema":                {SchemaVersion: 2, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("x"))},
		"scalar missing":        {SchemaVersion: 1, Kind: "scalar"},
		"scalar with set items": {SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("x")), Values: []apicontract.TypedValue{apicontract.NewStringValue("y")}},
		"scalar invalid":        {SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.TypedValue{})},
		"unknown kind":          {SchemaVersion: 1, Kind: "other"},
		"set overlap":           {SchemaVersion: 1, Kind: "set", Values: []apicontract.TypedValue{apicontract.NewStringValue("x")}, KeyFields: []string{"id"}},
		"set invalid value":     {SchemaVersion: 1, Kind: "set", Values: []apicontract.TypedValue{{}}},
		"duplicate fields":      {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id", "id"}},
		"invalid field":         {SchemaVersion: 1, Kind: "set", KeyFields: []string{"bad field"}},
		"empty key shape":       {SchemaVersion: 1, Kind: "set", Keys: []EntityKey{{Components: []KeyComponent{{Field: "id", Value: apicontract.NewStringValue("x")}}}}},
		"wrong key field":       {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "other", Value: apicontract.NewStringValue("x")}}}}},
		"invalid key value":     {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "id"}}}}},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateValue(value); !errors.Is(err, ErrInvalid) {
				t.Fatalf("validateValue = %v", err)
			}
		})
	}
	for name, raw := range map[string]json.RawMessage{
		"empty":              nil,
		"oversized":          json.RawMessage(`"` + strings.Repeat("x", 128*1024) + `"`),
		"invalid":            json.RawMessage("{"),
		"wrong kind":         testDefinition("dashboard"),
		"credential key":     json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[],"apiKey":"secret"}`),
		"duplicate query ID": json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[{"id":"q","queryType":"SQL","text":"select 1"},{"id":"q","queryType":"SQL","text":"select 2"}]}`),
	} {
		t.Run("definition "+name, func(t *testing.T) {
			if _, _, err := canonicalDefinitionDocument("query", raw); !errors.Is(err, ErrInvalid) {
				t.Fatalf("canonical definition error = %v", err)
			}
		})
	}
}

func TestValidateScopedRefsPropagatesOwnershipAndRevisionErrors(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "scoped-ref-errors", "scoped-ref-bag")
	put := validPutCommand(created, created.SessionID, "scoped-ref-bag")
	put.EntryID, put.RevisionID = "scoped-ref-entry", "scoped-ref-revision"
	executeOK(t, base, p, put)
	var session Session
	var bag Bag
	if err := base.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Sessions, created.SessionID, &session); e != nil {
			return e
		}
		_, e := r.Load(ctx, Bags, "scoped-ref-bag", &bag)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	ref := []EntryRef{{EntryID: "scoped-ref-entry", RevisionID: "scoped-ref-revision"}}
	for name, fault := range map[string]func(string, string) error{
		"entry": func(collection, id string) error {
			if collection == Entries {
				return ErrUnavailable
			}
			return nil
		},
		"bag": func(collection, id string) error {
			if collection == Bags {
				return ErrUnavailable
			}
			return nil
		},
		"revision": func(collection, id string) error {
			if collection == Revisions {
				return ErrUnavailable
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			var got error
			if err := base.repo.View(ctx, func(r Reader) error {
				_, got = base.validateScopedRefs(ctx, interceptedReader{Reader: r, failLoad: fault}, p, session, bag.Scope, ref)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(got, ErrUnavailable) {
				t.Fatalf("scoped ref error = %v", got)
			}
		})
	}
	var got error
	if err := base.repo.View(ctx, func(r Reader) error {
		_, got = base.validateScopedRefs(ctx, r, p, session, Scope{EnvironmentID: "prod", IdentityDomain: "tenant-a"}, ref)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(got, ErrForbidden) {
		t.Fatalf("scope mismatch = %v", got)
	}
}

func TestValidateRefsRejectsMalformedAndStaleReferences(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "validate-refs", "validate-refs-bag")
	put := validPutCommand(created, created.SessionID, "validate-refs-bag")
	put.EntryID, put.RevisionID = "validate-entry", "validate-revision"
	executeOK(t, base, p, put)
	var session Session
	var bag Bag
	if err := base.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Sessions, created.SessionID, &session); e != nil {
			return e
		}
		_, e := r.Load(ctx, Bags, "validate-refs-bag", &bag)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	ref := EntryRef{EntryID: put.EntryID, RevisionID: put.RevisionID}
	reader := func(errorsByKey map[string]error, entries map[string]any, revisions map[string]any) singleRecordReader {
		records := map[string]any{Entries + "/validate-entry": Entry{ID: "validate-entry", SessionID: session.ID, BagID: bag.ID, CurrentRevisionID: "validate-revision"}, Revisions + "/validate-revision": EntryRevision{ID: "validate-revision", EntryID: "validate-entry", BagID: bag.ID}}
		for key, value := range entries {
			records[key] = value
		}
		for key, value := range revisions {
			records[key] = value
		}
		return singleRecordReader{records: records, versions: map[string]int64{Entries + "/validate-entry": 1, Revisions + "/validate-revision": 1}, loadErrors: errorsByKey}
	}
	for name, test := range map[string]struct {
		reader  Reader
		refs    []EntryRef
		current bool
		want    error
	}{
		"too many refs":         {singleRecordReader{}, make([]EntryRef, 1001), false, ErrInvalid},
		"unsafe ref":            {singleRecordReader{}, []EntryRef{{EntryID: "bad id", RevisionID: "r1"}}, false, ErrInvalid},
		"conflicting duplicate": {reader(nil, nil, nil), []EntryRef{ref, {EntryID: ref.EntryID, RevisionID: "other-revision"}}, false, ErrInvalid},
		"entry unavailable":     {reader(map[string]error{Entries + "/validate-entry": ErrUnavailable}, nil, nil), []EntryRef{ref}, false, ErrUnavailable},
		"entry foreign bag":     {reader(nil, map[string]any{Entries + "/validate-entry": Entry{ID: ref.EntryID, SessionID: session.ID, BagID: "foreign"}}, nil), []EntryRef{ref}, false, ErrConflict},
		"stale current":         {reader(nil, map[string]any{Entries + "/validate-entry": Entry{ID: ref.EntryID, SessionID: session.ID, BagID: bag.ID, CurrentRevisionID: "another-revision"}}, nil), []EntryRef{ref}, true, ErrConflict},
		"revision unavailable":  {reader(map[string]error{Revisions + "/validate-revision": ErrUnavailable}, nil, nil), []EntryRef{ref}, false, ErrUnavailable},
		"revision foreign":      {reader(nil, nil, map[string]any{Revisions + "/validate-revision": EntryRevision{ID: ref.RevisionID, EntryID: ref.EntryID, BagID: "foreign"}}), []EntryRef{ref}, false, ErrForbidden},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := base.validateRefs(ctx, test.reader, p, session, bag, test.refs, test.current)
			if !errors.Is(err, test.want) {
				t.Fatalf("validateRefs = %v, want %v", err, test.want)
			}
		})
	}
	// Deliberately reverse a valid pair so the comparator's ordering path is covered.
	secondEntry := Entry{ID: "validate-entry-z", SessionID: session.ID, BagID: bag.ID, CurrentRevisionID: "validate-revision-z"}
	secondRevision := EntryRevision{ID: secondEntry.CurrentRevisionID, EntryID: secondEntry.ID, BagID: bag.ID}
	bag.EntryIDs = append(bag.EntryIDs, secondEntry.ID)
	r := reader(nil, map[string]any{Entries + "/" + secondEntry.ID: secondEntry}, map[string]any{Revisions + "/" + secondRevision.ID: secondRevision})
	refs, err := base.validateRefs(ctx, r, p, session, bag, []EntryRef{{EntryID: secondEntry.ID, RevisionID: secondRevision.ID}, ref}, false)
	if err != nil || refs[0].EntryID > refs[1].EntryID {
		t.Fatalf("sorted refs = %#v, %v", refs, err)
	}
}

func TestHistoricalReadersPropagateAuthorizationAndStorageFailures(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "historical-reader-errors", "historical-reader-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := validPutCommand(created, created.SessionID, "historical-reader-bag")
	put.EntryID, put.RevisionID = "historical-reader-entry", "historical-reader-revision"
	putReceipt := executeOK(t, base, p, put)
	pin := validPinCommand(putReceipt, created.SessionID, "historical-reader-run", "historical-reader-input", scope, "query")
	pin.Attachments = []EntryRef{{EntryID: put.EntryID, RevisionID: put.RevisionID}}
	executeOK(t, base, p, pin)
	for name, fault := range map[string]func(string, string) error{
		"input session": func(c, id string) error {
			if c == Sessions && id == created.SessionID {
				return ErrUnavailable
			}
			return nil
		},
		"input revision": func(c, id string) error {
			if c == Revisions && id == put.RevisionID {
				return ErrUnavailable
			}
			return nil
		},
		"input bag": func(c, id string) error {
			if c == Bags && id == "historical-reader-bag" {
				return ErrUnavailable
			}
			return nil
		},
		"run session": func(c, id string) error {
			if c == Sessions && id == created.SessionID {
				return ErrUnavailable
			}
			return nil
		},
		"run input": func(c, id string) error {
			if c == Inputs && id == "historical-reader-input" {
				return ErrUnavailable
			}
			return nil
		},
		"operation session": func(c, id string) error {
			if c == Sessions && id == created.SessionID {
				return ErrUnavailable
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := interceptService(t, base, p, &interceptedRepository{Repository: base.repo, failLoad: fault})
			switch name {
			case "input session", "input revision", "input bag":
				if _, _, err := svc.ReadInput(ctx, p, "historical-reader-input"); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("ReadInput = %v", err)
				}
			case "run session", "run input":
				if _, err := svc.ReadRun(ctx, p, "historical-reader-run"); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("ReadRun = %v", err)
				}
			case "operation session":
				if _, err := svc.ReadOperation(ctx, p, "pin-historical-reader-run"); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("ReadOperation = %v", err)
				}
			}
		})
	}
	for name, call := range map[string]func(*Service) error{
		"input authorization":     func(s *Service) error { _, _, e := s.ReadInput(ctx, p, "historical-reader-input"); return e },
		"run authorization":       func(s *Service) error { _, e := s.ReadRun(ctx, p, "historical-reader-run"); return e },
		"operation authorization": func(s *Service) error { _, e := s.ReadOperation(ctx, p, "pin-historical-reader-run"); return e },
	} {
		t.Run(name, func(t *testing.T) {
			svc := interceptService(t, base, p, nil)
			svc.authorize = func(context.Context, Access) error { return ErrForbidden }
			if err := call(svc); !errors.Is(err, ErrForbidden) {
				t.Fatalf("authorization = %v", err)
			}
		})
	}
}

func TestSnapshotRejectsUnboundedAndMismatchedRecords(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "snapshot-corrupt", "snapshot-corrupt-bag")
	var session Session
	if err := base.repo.View(ctx, func(r Reader) error { _, e := r.Load(ctx, Sessions, created.SessionID, &session); return e }); err != nil {
		t.Fatal(err)
	}
	session.BagIDs = make([]string, 65)
	for i := range session.BagIDs {
		session.BagIDs[i] = "snapshot-corrupt-bag"
	}
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Sessions, session.ID, session, session.Version, p.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(ctx, p, session.ID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bag count limit = %v", err)
	}

	base, p = newTestService(t, nil)
	created = createTestSession(t, base, p, "snapshot-mismatch", "snapshot-mismatch-bag")
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Bags, "snapshot-mismatch-bag", Bag{ID: "snapshot-mismatch-bag", SessionID: "another-session", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Version: 1}, 1, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(ctx, p, created.SessionID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("bag session mismatch = %v", err)
	}

	base, p = newTestService(t, nil)
	created = createTestSession(t, base, p, "snapshot-revision-mismatch", "snapshot-revision-bag")
	put := validPutCommand(created, "snapshot-revision-mismatch", "snapshot-revision-bag")
	put.EntryID, put.RevisionID = "snapshot-revision-entry", "snapshot-revision"
	executeOK(t, base, p, put)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Revisions, "snapshot-revision", EntryRevision{ID: "snapshot-revision", EntryID: "snapshot-revision-entry", BagID: "other-bag"}, 1, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(ctx, p, created.SessionID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revision bag mismatch = %v", err)
	}

	base, p = newTestService(t, nil)
	created = createTestSession(t, base, p, "snapshot-entry-mismatch", "snapshot-entry-bag")
	put = validPutCommand(created, created.SessionID, "snapshot-entry-bag")
	put.EntryID, put.RevisionID = "snapshot-entry", "snapshot-entry-revision"
	executeOK(t, base, p, put)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Entries, "snapshot-entry", Entry{ID: "snapshot-entry", SessionID: created.SessionID, BagID: "different-bag", CurrentRevisionID: "snapshot-entry-revision"}, 1, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(ctx, p, created.SessionID); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("entry bag mismatch = %v", err)
	}
}

func TestSnapshotByteBudgetCoversEveryReadLayer(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "snapshot-byte-layers", "snapshot-byte-bag")
	ctx := context.Background()
	var session Session
	var set AppliedContextSet
	var bag Bag
	if err := base.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Sessions, created.SessionID, &session); e != nil {
			return e
		}
		if _, e := r.Load(ctx, ContextSets, session.ContextSetID, &set); e != nil {
			return e
		}
		_, e := r.Load(ctx, Bags, "snapshot-byte-bag", &bag)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", MaxSnapshotBytes+1)
	tests := []struct {
		name   string
		mutate func(*Session, *AppliedContextSet, *Bag)
	}{
		{"session", func(s *Session, _ *AppliedContextSet, _ *Bag) { s.Title = large }},
		{"context set", func(_ *Session, set *AppliedContextSet, _ *Bag) {
			set.Slots = []AppliedSlot{{Scope: Scope{EnvironmentID: large}}}
		}},
		{"bag", func(_ *Session, _ *AppliedContextSet, bag *Bag) { bag.Title = large }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s, cs, b := session, set, bag
			test.mutate(&s, &cs, &b)
			r := singleRecordReader{records: map[string]any{ContextSets + "/" + s.ContextSetID: cs, Bags + "/" + b.ID: b}, versions: map[string]int64{ContextSets + "/" + s.ContextSetID: 1, Bags + "/" + b.ID: 1}}
			if _, err := base.loadSnapshot(ctx, r, p, s, "read"); !errors.Is(err, errSnapshotLimit) {
				t.Fatalf("loadSnapshot budget error = %v", err)
			}
		})
	}
	if _, err := addSnapshotBytes(0, make(chan int)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unencodable snapshot value = %v", err)
	}
	if _, err := addSnapshotBytes(MaxSnapshotBytes, "x"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("snapshot overflow = %v", err)
	}
}

func TestLoadSnapshotStopsBeforeEntryBeyondCap(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	session := Session{ID: "bounded-snapshot", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, ContextSetID: "bounded-set", BagIDs: []string{"bounded-bag"}}
	entries := make([]string, MaxSnapshotEntries+1)
	for i := range entries {
		entries[i] = "bounded-entry"
	}
	bag := Bag{ID: "bounded-bag", SessionID: session.ID, Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Version: 1, EntryIDs: entries}
	set := AppliedContextSet{ID: "bounded-set", SessionID: session.ID}
	entry := Entry{ID: "bounded-entry", SessionID: session.ID, BagID: bag.ID, CurrentRevisionID: "bounded-revision"}
	revision := EntryRevision{ID: "bounded-revision", EntryID: entry.ID, BagID: bag.ID}
	reader := singleRecordReader{
		records: map[string]any{
			ContextSets + "/" + set.ID:    set,
			Bags + "/" + bag.ID:           bag,
			Entries + "/" + entry.ID:      entry,
			Revisions + "/" + revision.ID: revision,
		},
		versions: map[string]int64{ContextSets + "/" + set.ID: 1, Bags + "/" + bag.ID: 1, Entries + "/" + entry.ID: 1, Revisions + "/" + revision.ID: 1},
	}
	if _, err := base.loadSnapshot(ctx, reader, p, session, "read"); !errors.Is(err, errSnapshotLimit) {
		t.Fatalf("entry count limit = %v", err)
	}
}

func TestRevisionReadersPropagateStorageAndOwnershipFailures(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "revision-read-errors", "revision-read-bag")
	put := validPutCommand(created, "revision-read-errors", "revision-read-bag")
	put.EntryID, put.RevisionID = "revision-read-entry", "revision-read-revision"
	executeOK(t, base, p, put)
	for name, fault := range map[string]func(string, string) error{
		"entry": func(collection, id string) error {
			if collection == Entries && id == "revision-read-entry" {
				return ErrUnavailable
			}
			return nil
		},
		"bag": func(collection, id string) error {
			if collection == Bags && id == "revision-read-bag" {
				return ErrUnavailable
			}
			return nil
		},
		"revision": func(collection, id string) error {
			if collection == Revisions && id == "revision-read-revision" {
				return ErrUnavailable
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			svc := interceptService(t, base, p, &interceptedRepository{Repository: base.repo, failLoad: fault})
			if _, err := svc.ReadEntryRevision(ctx, p, created.SessionID, "revision-read-entry", "revision-read-revision"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("read error = %v", err)
			}
		})
	}
	sessionFailure := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == Sessions && id == created.SessionID {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, base, p, sessionFailure).ReadEntryRevision(ctx, p, created.SessionID, "revision-read-entry", "revision-read-revision"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("session read error = %v", err)
	}
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Revisions, "wrong-revision-id", EntryRevision{ID: "different-id", EntryID: "revision-read-entry", BagID: "revision-read-bag"}, 0, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadEntryRevision(ctx, p, created.SessionID, "revision-read-entry", "wrong-revision-id"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revision ID mismatch = %v", err)
	}
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Entries, "foreign-entry", Entry{ID: "foreign-entry", SessionID: "another-session", BagID: "revision-read-bag"}, 0, p.Actor, "another-session")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadEntryRevision(ctx, p, created.SessionID, "foreign-entry", "revision-read-revision"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("entry session mismatch = %v", err)
	}
}

func TestPageReaderViewFailuresAndFilteredHistoryRows(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	if _, err := interceptService(t, base, p, &interceptedRepository{Repository: base.repo, viewErr: ErrUnavailable}).ListSessions(ctx, p, "", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListSessions view failure = %v", err)
	}
	deniedList := interceptService(t, base, p, nil)
	deniedList.authorize = func(context.Context, Access) error { return ErrForbidden }
	if _, err := deniedList.ListSessions(ctx, p, "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListSessions authorization failure = %v", err)
	}
	if _, err := interceptService(t, base, p, &interceptedRepository{Repository: base.repo, viewErr: ErrUnavailable}).ReadOperation(ctx, p, "valid-op"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ReadOperation view failure = %v", err)
	}

	created := createTestSession(t, base, p, "filtered-history", "filtered-history-bag")
	put := validPutCommand(created, "filtered-history", "filtered-history-bag")
	put.EntryID, put.RevisionID = "filtered-entry", "filtered-revision"
	putReceipt := executeOK(t, base, p, put)
	otherPut := put
	otherPut.OpID, otherPut.ExpectedVersion, otherPut.ExpectedBagVersion = "filtered-other-put", putReceipt.SessionVersion, 2
	otherPut.EntryID, otherPut.RevisionID = "other-entry", "other-revision"
	otherReceipt := executeOK(t, base, p, otherPut)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Entries, "entry-outside-session", Entry{ID: "entry-outside-session", SessionID: "another-session", BagID: "filtered-history-bag"}, 0, p.Actor, "another-session")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ListEntryRevisions(ctx, p, created.SessionID, "entry-outside-session", "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign history entry = %v", err)
	}
	page, err := base.ListEntryRevisions(ctx, p, created.SessionID, "filtered-entry", "", 10)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "filtered-revision" {
		t.Fatalf("filtered revision rows = %+v, %v", page, err)
	}
	// The revision reader must reject a matching entry revision whose bag scope
	// disagrees with the entry's retained identity.
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Revisions, "wrong-bag-revision", EntryRevision{ID: "wrong-bag-revision", EntryID: "filtered-entry", BagID: "foreign-bag"}, 0, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = base.ListEntryRevisions(ctx, p, created.SessionID, "filtered-entry", "", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched history bag = %v", err)
	}

	// A user's page must skip other sessions' rows while retaining its own runs
	// and messages.
	second := createTestSession(t, base, p, "other-history-session", "other-history-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	pin := validPinCommand(second, "other-history-session", "other-history-run", "other-history-input", scope, "chat")
	pin.Text = "question"
	executeOK(t, base, p, pin)
	if _, err = base.ListRuns(ctx, p, created.SessionID, "", 10); err != nil {
		t.Fatalf("filtered run page: %v", err)
	}
	if _, err = base.ListMessages(ctx, p, created.SessionID, "", 10); err != nil {
		t.Fatalf("filtered message page: %v", err)
	}
	if _, err = base.Execute(ctx, p, Command{OpID: "filtered-history-report", Action: "reportClientRun", SessionID: second.SessionID, ExpectedRunVersion: 1, RunID: "other-history-run", Kind: "chat", RunState: "completed", Text: "answer"}); err != nil {
		t.Fatal(err)
	}
	legacy := &LegacyImport{SourceID: "filtered-legacy", Scope: scope, Messages: []LegacyChatMessage{{ID: "legacy-message", Role: "user", Text: "imported question"}}}
	if _, err = base.Execute(ctx, p, Command{OpID: "filtered-legacy-op", Action: "importLegacy", LegacyImport: legacy}); err != nil {
		t.Fatal(err)
	}
	legacyManifest, _, _, err := prepareLegacyImport(p, legacy)
	if err != nil {
		t.Fatal(err)
	}
	denied := interceptService(t, base, p, nil)
	denied.authorize = func(_ context.Context, a Access) error {
		if a.Action == "listMessages" && a.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := denied.ListMessages(ctx, p, legacyManifest.SessionID, "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("legacy message scope revocation = %v", err)
	}
	_ = otherReceipt
}

type sequenceReader struct {
	rows []json.RawMessage
}

func (r sequenceReader) Load(context.Context, string, string, any) (int64, error) {
	return 0, ErrNotFound
}

func (r sequenceReader) List(_ context.Context, _ string, _ string, cursor string, limit int) ([]json.RawMessage, string, error) {
	if limit < 0 {
		return nil, "", ErrInvalid
	}
	start := 0
	if cursor != "" {
		for start < len(r.rows) {
			var item struct {
				ID string `json:"id"`
			}
			_ = json.Unmarshal(r.rows[start], &item)
			if item.ID > cursor {
				break
			}
			start++
		}
	}
	if start >= len(r.rows) {
		return nil, "", nil
	}
	if limit == 0 {
		return r.rows[start : start+1], cursor, nil
	}
	end := min(start+limit, len(r.rows))
	next := ""
	if end < len(r.rows) {
		var item struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(r.rows[end-1], &item)
		next = item.ID
	}
	return r.rows[start:end], next, nil
}

func TestListPageStopsAtScanBudgetWithProgressCursor(t *testing.T) {
	rows := make([]json.RawMessage, 1001)
	for i := range rows {
		rows[i] = json.RawMessage(fmt.Sprintf(`{"id":"row-%04d"}`, i))
	}
	page, err := listPage[Session](context.Background(), overLimitReader{rows: rows}, Sessions, "alice", "", 10, func(v Session) (string, bool, error) {
		return v.ID, false, nil
	})
	if err != nil || len(page.Items) != 0 || !page.HasMore || page.NextCursor != "row-0999" {
		t.Fatalf("bounded scan = %+v, %v", page, err)
	}
	_, err = listPage[Session](context.Background(), sequenceReader{rows: rows[:1]}, Sessions, "alice", "", 10, func(v Session) (string, bool, error) {
		return "different-id", true, nil
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("mismatched record ID = %v", err)
	}
}

type overLimitReader struct{ rows []json.RawMessage }

func (r overLimitReader) Load(context.Context, string, string, any) (int64, error) {
	return 0, ErrNotFound
}

func (r overLimitReader) List(context.Context, string, string, string, int) ([]json.RawMessage, string, error) {
	return r.rows, "", nil
}

func TestPageReadersRejectMalformedRowsAndStorageFailures(t *testing.T) {
	base, p := newTestService(t, nil)
	if _, err := base.ListSessions(context.Background(), p, "", 10); err != nil {
		t.Fatal(err)
	}
	fault := &interceptedRepository{Repository: base.repo, failList: func(string, string, string, int) ([]json.RawMessage, string, error) { return nil, "", ErrUnavailable }}
	if _, err := interceptService(t, base, p, fault).ListSessions(context.Background(), p, "", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("list failure: %v", err)
	}
	for name, override := range map[string]func(string, string, string, int) ([]json.RawMessage, string, error){
		"malformed JSON": func(string, string, string, int) ([]json.RawMessage, string, error) {
			return []json.RawMessage{json.RawMessage("{")}, "", nil
		},
		"missing ID": func(string, string, string, int) ([]json.RawMessage, string, error) {
			return []json.RawMessage{json.RawMessage(`{"owner":"alice"}`)}, "", nil
		},
		"empty rows with cursor": func(string, string, string, int) ([]json.RawMessage, string, error) { return nil, "stuck", nil },
		"one row repeated cursor": func(_ string, _ string, cursor string, _ int) ([]json.RawMessage, string, error) {
			return []json.RawMessage{json.RawMessage(`{"id":"one","owner":"alice","databaseId":"db1","projectId":"project1"}`)}, "one", nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := &interceptedRepository{Repository: base.repo, listOverride: override}
			cursor := ""
			if name == "one row repeated cursor" {
				cursor = "one"
			}
			_, err := interceptService(t, base, p, r).ListSessions(context.Background(), p, cursor, 10)
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("malformed page error = %v", err)
			}
		})
	}
}

func TestDALSaveAndDecodeRejectUnencodableValues(t *testing.T) {
	base, p := newTestService(t, nil)
	repo := base.repo.(*DALRepository)
	if err := repo.Transact(context.Background(), func(tx Transaction) error {
		d := tx.(*dalTransaction)
		if err := d.Save(context.Background(), Sessions, "encode-fail", func() {}, 0, p.Actor, "encode-fail"); !errors.Is(err, ErrInvalid) {
			return fmt.Errorf("marshal failure: %w", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRecord(Sessions, "id", map[string]any{"payload": "{}", "owner": "alice", "recordId": "id", "sessionId": "s", "version": make(chan int)}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("version marshal failure = %v", err)
	}
}

func TestHistoryReadersReauthorizeAndPropagateCorruptReferences(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "history-session", "history-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "history-put", Action: "putEntry", SessionID: "history-session", ExpectedVersion: created.SessionVersion, BagID: "history-bag", ExpectedBagVersion: 1, EntryID: "history-entry", RevisionID: "history-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	pin := Command{OpID: "history-pin", Action: "pinInput", SessionID: "history-session", ExpectedVersion: putReceipt.SessionVersion, InputSnapshotID: "history-input", RunID: "history-run", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Scope: &scope, Attachments: []EntryRef{{EntryID: "history-entry", RevisionID: "history-revision"}}, Text: "question"}
	executeOK(t, base, p, pin)
	_, err := base.Execute(context.Background(), p, Command{OpID: "history-report", Action: "reportClientRun", SessionID: "history-session", ExpectedRunVersion: 1, RunID: "history-run", Kind: "chat", RunState: "completed", Text: "answer"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := base.ListEntryRevisions(context.Background(), p, "history-session", "history-entry", "", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadEntryRevision(context.Background(), p, "history-session", "history-entry", "history-revision"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := base.ReadInput(context.Background(), p, "history-input"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ReadRun(context.Background(), p, "history-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ListRuns(context.Background(), p, "history-session", "", 10); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ListMessages(context.Background(), p, "history-session", "", 10); err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func(*Service) error{
		"revision list load": func(s *Service) error {
			_, e := s.ListEntryRevisions(context.Background(), p, "history-session", "history-entry", "", 10)
			return e
		},
		"revision read load": func(s *Service) error {
			_, e := s.ReadEntryRevision(context.Background(), p, "history-session", "history-entry", "history-revision")
			return e
		},
		"input read load": func(s *Service) error { _, _, e := s.ReadInput(context.Background(), p, "history-input"); return e },
		"run read load":   func(s *Service) error { _, e := s.ReadRun(context.Background(), p, "history-run"); return e },
		"run list input load": func(s *Service) error {
			_, e := s.ListRuns(context.Background(), p, "history-session", "", 10)
			return e
		},
		"message list input load": func(s *Service) error {
			_, e := s.ListMessages(context.Background(), p, "history-session", "", 10)
			return e
		},
		"revision list session load": func(s *Service) error {
			_, e := s.ListEntryRevisions(context.Background(), p, "history-session", "history-entry", "", 10)
			return e
		},
		"revision list bag load": func(s *Service) error {
			_, e := s.ListEntryRevisions(context.Background(), p, "history-session", "history-entry", "", 10)
			return e
		},
		"input read bag load":   func(s *Service) error { _, _, e := s.ReadInput(context.Background(), p, "history-input"); return e },
		"run read session load": func(s *Service) error { _, e := s.ReadRun(context.Background(), p, "history-run"); return e },
		"message list session load": func(s *Service) error {
			_, e := s.ListMessages(context.Background(), p, "history-session", "", 10)
			return e
		},
		"operation load":         func(s *Service) error { _, e := s.ReadOperation(context.Background(), p, "history-put"); return e },
		"operation session load": func(s *Service) error { _, e := s.ReadOperation(context.Background(), p, "history-put"); return e },
		"list runs session load": func(s *Service) error {
			_, e := s.ListRuns(context.Background(), p, "history-session", "", 10)
			return e
		},
	} {
		t.Run(name, func(t *testing.T) {
			faults := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
				switch name {
				case "revision list session load", "run read session load", "message list session load", "operation session load", "list runs session load":
					if collection == Sessions && id == "history-session" {
						return ErrUnavailable
					}
				case "operation load":
					if collection == Operations {
						return ErrUnavailable
					}
				case "revision list bag load", "input read bag load":
					if collection == Bags && id == "history-bag" {
						return ErrUnavailable
					}
				}
				if collection == Inputs && id == "history-input" && (name == "input read load" || name == "run list input load" || name == "message list input load") {
					return ErrUnavailable
				}
				if collection == Revisions && id == "history-revision" && (name == "revision list load" || name == "revision read load" || name == "input read load") {
					return ErrUnavailable
				}
				if collection == Entries && id == "history-entry" && (name == "revision list load" || name == "revision read load") {
					return ErrUnavailable
				}
				if collection == Runs && id == "history-run" && name == "run read load" {
					return ErrUnavailable
				}
				return nil
			}}
			if err := call(interceptService(t, base, p, faults)); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	deny := interceptService(t, base, p, nil)
	deny.authorize = func(_ context.Context, a Access) error {
		if a.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := deny.ReadOperation(context.Background(), p, "history-put"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("operation scope revocation = %v", err)
	}
	if _, _, err := deny.ReadInput(context.Background(), p, "history-input"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("input scope revocation = %v", err)
	}
	if _, err := deny.ReadRun(context.Background(), p, "history-run"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("run scope revocation = %v", err)
	}
	if _, err := deny.ListRuns(context.Background(), p, "history-session", "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("run list scope revocation = %v", err)
	}
	if _, err := deny.ListMessages(context.Background(), p, "history-session", "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("message list scope revocation = %v", err)
	}
}

func TestReferenceValidatorsRejectStaleForeignAndDuplicatePins(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "refs-session", "refs-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "refs-put", Action: "putEntry", SessionID: "refs-session", ExpectedVersion: created.SessionVersion, BagID: "refs-bag", ExpectedBagVersion: 1, EntryID: "refs-entry", RevisionID: "refs-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	var session Session
	var bag Bag
	if err := base.repo.View(context.Background(), func(r Reader) error {
		_, e := r.Load(context.Background(), Sessions, "refs-session", &session)
		if e != nil {
			return e
		}
		_, e = r.Load(context.Background(), Bags, "refs-bag", &bag)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	reader := base.repo.(*DALRepository)
	assertRef := func(name string, refs []EntryRef, expected error) {
		t.Helper()
		err := reader.View(context.Background(), func(r Reader) error {
			_, e := base.validateRefs(context.Background(), r, p, session, bag, refs, false)
			return e
		})
		if !errors.Is(err, expected) {
			t.Fatalf("%s: got %v, want %v", name, err, expected)
		}
	}
	assertRef("unsafe", []EntryRef{{EntryID: "bad id", RevisionID: "refs-revision"}}, ErrInvalid)
	assertRef("missing", []EntryRef{{EntryID: "missing-entry", RevisionID: "refs-revision"}}, ErrNotFound)
	assertRef("foreign", []EntryRef{{EntryID: "refs-entry", RevisionID: "wrong-revision"}}, ErrNotFound)
	assertRef("duplicate same revision", []EntryRef{{EntryID: "refs-entry", RevisionID: "refs-revision"}, {EntryID: "refs-entry", RevisionID: "refs-revision"}}, nil)
	assertRef("duplicate changed revision", []EntryRef{{EntryID: "refs-entry", RevisionID: "refs-revision"}, {EntryID: "refs-entry", RevisionID: "wrong-revision"}}, ErrInvalid)
	assertRef("too many", make([]EntryRef, 1001), ErrInvalid)
	if err := reader.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Revisions, "foreign-revision", EntryRevision{ID: "foreign-revision", EntryID: "someone-else", BagID: "refs-bag"}, 0, p.Actor, "refs-session")
	}); err != nil {
		t.Fatal(err)
	}
	assertRef("revision ownership mismatch", []EntryRef{{EntryID: "refs-entry", RevisionID: "foreign-revision"}}, ErrForbidden)
	assertRef("bag membership mismatch", []EntryRef{{EntryID: "refs-entry", RevisionID: "refs-revision"}}, nil)
	secondPut := validPutCommand(putReceipt, "refs-session", "refs-bag")
	secondPut.OpID, secondPut.EntryID, secondPut.RevisionID, secondPut.ExpectedBagVersion = "refs-second-put", "refs-entry-2", "refs-revision-2", 2
	secondReceipt := executeOK(t, base, p, secondPut)
	if err := reader.View(context.Background(), func(r Reader) error {
		_, err := base.validateScopedRefs(context.Background(), r, p, session, scope, []EntryRef{{EntryID: "refs-entry-2", RevisionID: "refs-revision-2"}, {EntryID: "refs-entry", RevisionID: "refs-revision"}})
		return err
	}); err != nil {
		t.Fatalf("scoped refs should validate and sort: %v", err)
	}
	_ = secondReceipt
	bag.EntryIDs = []string{}
	assertRef("entry absent from bag", []EntryRef{{EntryID: "refs-entry", RevisionID: "refs-revision"}}, ErrConflict)
	_ = putReceipt
	_ = scope
}

func TestInputSnapshotAuthorizationAndScopeIntegrity(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "input-scope-session", "input-scope-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "input-scope-put", Action: "putEntry", SessionID: "input-scope-session", ExpectedVersion: created.SessionVersion, BagID: "input-scope-bag", ExpectedBagVersion: 1, EntryID: "input-scope-entry", RevisionID: "input-scope-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	pin := Command{OpID: "input-scope-pin", Action: "pinInput", SessionID: "input-scope-session", ExpectedVersion: putReceipt.SessionVersion, InputSnapshotID: "input-scope-input", RunID: "input-scope-run", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Scope: &scope, Attachments: []EntryRef{{EntryID: "input-scope-entry", RevisionID: "input-scope-revision"}}}
	executeOK(t, base, p, pin)
	if err := base.repo.Transact(context.Background(), func(tx Transaction) error {
		var input InputSnapshot
		if _, err := tx.Load(context.Background(), Inputs, "input-scope-input", &input); err != nil {
			return err
		}
		input.Scope.IdentityDomain = "another-tenant"
		return tx.Save(context.Background(), Inputs, input.ID, input, 1, p.Actor, input.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := base.ReadInput(context.Background(), p, "input-scope-input"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("wrong source scope must fail closed: %v", err)
	}
}

func TestExecuteRejectsMalformedScopeReplayAndStateTransitions(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	created := createTestSession(t, base, p, "transitions", "transitions-bag")
	for name, command := range map[string]Command{
		"unsafe operation":         {OpID: "bad id", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID},
		"unsafe session":           {OpID: "op", Action: "createSession", SessionID: "bad id", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID},
		"wrong database":           {OpID: "db-mismatch", Action: "createSession", SessionID: "s", DatabaseID: "other", ProjectID: p.ProjectID},
		"wrong project":            {OpID: "project-mismatch", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: "other"},
		"unknown action":           {OpID: "unknown-action", Action: "notAnAction", SessionID: "transitions", ExpectedVersion: created.SessionVersion},
		"private params elsewhere": {OpID: "wrong-params", Action: "transitionSession", SessionID: "s", ResolvedParameters: map[string]apicontract.TypedValue{"p": apicontract.NewStringValue("secret")}},
		"definition elsewhere":     {OpID: "wrong-definition", Action: "transitionSession", SessionID: "s", DefinitionDocument: testDefinition("chat")},
		"execution elsewhere":      {OpID: "wrong-execution", Action: "transitionSession", SessionID: "s", ExecutionDocument: testQueryDefinition("select 1")},
		"invalid document digest":  {OpID: "digest-failure", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, DefinitionDocument: json.RawMessage("{")},
	} {
		if _, err := base.Execute(ctx, p, command); !errors.Is(err, ErrInvalid) && name != "wrong database" && name != "wrong project" {
			t.Errorf("%s: got %v", name, err)
		}
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "wrong-database", Action: "createSession", SessionID: "s", DatabaseID: "other", ProjectID: p.ProjectID}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("database mismatch = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "wrong-project", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: "other"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("project mismatch = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "denied", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "b", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "denied", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "b", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); err != nil {
		t.Fatalf("same operation replay failed: %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "create-existing", Action: "createSession", SessionID: "s", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "b", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); !errors.Is(err, ErrConflict) {
		t.Fatalf("session ID collision = %v", err)
	}

	fault := &interceptedRepository{Repository: base.repo, transactErr: errors.New("storage down")}
	if _, err := interceptService(t, base, p, fault).Execute(ctx, p, Command{OpID: "transact-fail", Action: "createSession", SessionID: "other", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "other-bag", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); err == nil || err.Error() != "storage down" {
		t.Fatalf("transaction failure = %v", err)
	}

	badCreate := Command{OpID: "bad-create", Action: "createSession", SessionID: "bad-create-session", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}
	if _, err := base.Execute(ctx, p, badCreate); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid create scope/IDs = %v", err)
	}
	for name, command := range map[string]Command{
		"stale version":      {OpID: "stale-transition", Action: "transitionSession", SessionID: "transitions", ExpectedVersion: created.SessionVersion - 1, Transition: "stop"},
		"unknown transition": {OpID: "unknown-transition", Action: "transitionSession", SessionID: "transitions", ExpectedVersion: created.SessionVersion, Transition: "unknown"},
	} {
		_, err := base.Execute(ctx, p, command)
		want := ErrConflict
		if name == "unknown transition" {
			want = ErrInvalid
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", name, err, want)
		}
	}
	archive := Command{OpID: "archive-transition", Action: "transitionSession", SessionID: "transitions", ExpectedVersion: created.SessionVersion, Transition: "archive"}
	archiveReceipt := executeOK(t, base, p, archive)
	if _, err := base.Execute(ctx, p, Command{OpID: "archived-mutation", Action: "createBag", SessionID: "transitions", ExpectedVersion: archiveReceipt.SessionVersion, BagID: "x", Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("archived session mutation = %v", err)
	}
	restore := Command{OpID: "restore-transition", Action: "transitionSession", SessionID: "transitions", ExpectedVersion: archiveReceipt.SessionVersion, Transition: "restore"}
	restored := executeOK(t, base, p, restore)
	if restored.SessionVersion != archiveReceipt.SessionVersion+1 {
		t.Fatalf("restore did not increment session version: %+v", restored)
	}
}

func TestExecutePinsInputsAndRejectsInvalidDefinitionAndRefs(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	created := createTestSession(t, base, p, "pin-edges", "pin-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	valid := Command{Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope}
	invalids := []Command{
		{OpID: "pin-id", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "bad id", RunID: "run", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope},
		{OpID: "pin-no-run", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "bad id", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope},
		{OpID: "pin-bad-scope", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query")},
		{OpID: "pin-bad-kind", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "unknown", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope},
		{OpID: "pin-empty-definition", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "", DefinitionRevision: "r1", DefinitionDocument: testDefinition("query"), Scope: &scope},
		{OpID: "pin-empty-revision", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "query", DefinitionRevision: "", DefinitionDocument: testDefinition("query"), Scope: &scope},
		{OpID: "pin-long-text", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "chat", DefinitionID: "assistant", DefinitionRevision: "r1", DefinitionDocument: testDefinition("chat"), Text: strings.Repeat("x", 100001), Scope: &scope},
		{OpID: "pin-bad-doc", Action: "pinInput", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: json.RawMessage("{}"), Scope: &scope},
	}
	for _, command := range invalids {
		if _, err := base.Execute(ctx, p, command); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: got %v", command.OpID, err)
		}
	}
	check := valid
	check.OpID, check.Kind, check.InputSnapshotID, check.RunID = "client-check-pin", "check", "check-input", "check-run"
	check.DefinitionDocument = testDefinition("check")
	if _, err := base.Execute(ctx, p, check); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client Check pin = %v", err)
	}
	valid.OpID, valid.InputSnapshotID, valid.RunID = "valid-pin", "valid-input", "valid-run"
	valid.Attachments = []EntryRef{{EntryID: "missing-entry", RevisionID: "missing-revision"}}
	if _, err := base.Execute(ctx, p, valid); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unresolved attachment = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "bad-apply", Action: "applyContext", SessionID: "pin-edges", ExpectedVersion: created.SessionVersion, BagID: "pin-bag", ExpectedBagVersion: 1, Scope: &scope, ExpectedContextSetID: "cs_" + opKey(p, "create-pin-edges")}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing apply enable flag = %v", err)
	}
}

func TestLegacyImportDeduplicatesAtomicallyAndPreservesProvenance(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	when1 := time.Date(2024, 2, 3, 4, 5, 6, 0, time.UTC)
	when2 := when1.Add(time.Minute)
	payload := &LegacyImport{SourceID: "legacy-record-42", Title: "Old session", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Entries: []LegacyEntry{{ID: "old-order", Alias: "Order", Subject: Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "mapping-old"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("9007199254740997"))}, CreatedAt: when1}}, Messages: []LegacyChatMessage{{ID: "turn-1", Role: "user", Text: "Old question", CreatedAt: when1}, {ID: "turn-2", Role: "assistant", Text: "Old answer", CreatedAt: when2}}}
	cmd := Command{OpID: "legacy-import-1", Action: "importLegacy", LegacyImport: payload}
	first, err := base.Execute(ctx, p, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if first.Receipt.SessionID == "" || first.Receipt.ResourceID != first.Receipt.SessionID || first.Receipt.SessionVersion != 1 || first.Receipt.ResourceVersion != 1 || first.Receipt.Scope == nil || *first.Receipt.Scope != payload.Scope {
		t.Fatalf("import receipt omitted the derived session/scope: %+v", first.Receipt)
	}
	assertPrivateFieldsAbsentFromOperationMetadata(t, base, p, cmd.OpID, payload.Title, "Old question", "Old answer", "9007199254740997", payload.SourceID)
	second := cmd
	second.OpID = "legacy-import-retry"
	retry, err := base.Execute(ctx, p, second)
	if err != nil {
		t.Fatalf("same source retry under a new operation ID: %v", err)
	}
	if retry.Receipt.SessionID != first.Receipt.SessionID || retry.Receipt.SessionVersion != first.Receipt.SessionVersion || retry.Receipt.ResourceVersion != first.Receipt.ResourceVersion {
		t.Fatalf("idempotent legacy import changed the session: first=%+v retry=%+v", first.Receipt, retry.Receipt)
	}

	snapshot, err := base.GetSnapshot(ctx, p, first.Receipt.SessionID)
	if err != nil || snapshot.Session.Title != payload.Title || snapshot.Session.State != "active" || len(snapshot.Bags) != 1 || len(snapshot.Entries) != 1 || len(snapshot.Revisions) != 1 || len(snapshot.ContextSet.Slots) != 0 {
		t.Fatalf("legacy snapshot not imported once: snapshot=%+v err=%v", snapshot, err)
	}
	if snapshot.Entries[0].Alias != "Order" || snapshot.Revisions[0].Value.Scalar == nil || snapshot.Revisions[0].Value.Scalar.Str != "9007199254740997" || snapshot.Revisions[0].Provenance != "legacy-import" || snapshot.Revisions[0].LegacySourceID != payload.SourceID || snapshot.Revisions[0].SourceDigest == "" || !snapshot.Revisions[0].CreatedAt.Equal(when1) {
		t.Fatalf("legacy value or provenance lost: %+v", snapshot.Revisions[0])
	}
	messages, err := base.ListMessages(ctx, p, first.Receipt.SessionID, "", 10)
	if err != nil || len(messages.Items) != 2 {
		t.Fatalf("historical turns are not readable without fabricated inputs: %+v err=%v", messages, err)
	}
	if messages.Items[0].Text != "Old question" || messages.Items[1].Text != "Old answer" || messages.Items[0].Role != "user" || messages.Items[1].Role != "assistant" || messages.Items[0].Provenance != "legacy-import" || messages.Items[0].SourceDigest == "" || !messages.Items[1].HistoricalAt.Equal(when2) {
		t.Fatalf("legacy chat history ordering or provenance lost: %+v", messages.Items)
	}
	runs, err := base.ListRuns(ctx, p, first.Receipt.SessionID, "", 10)
	if err != nil || len(runs.Items) != 0 {
		t.Fatalf("legacy history fabricated execution runs: %+v err=%v", runs, err)
	}

	changed := *payload
	changed.Title = "Edited old session"
	modified := cmd
	modified.OpID, modified.LegacyImport = "legacy-import-changed", &changed
	if _, err := base.Execute(ctx, p, modified); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed source digest should conflict: %v", err)
	}
	otherScope := *payload
	otherScope.Scope.IdentityDomain = "tenant-b"
	other := cmd
	other.OpID, other.LegacyImport = "legacy-import-other-scope", &otherScope
	separate, err := base.Execute(ctx, p, other)
	if err != nil || separate.Receipt.SessionID == first.Receipt.SessionID {
		t.Fatalf("separate environment scope was not isolated: receipt=%+v err=%v", separate.Receipt, err)
	}
	otherActor := p
	otherActor.Actor = "bob"
	otherActorImport, err := base.Execute(ctx, otherActor, Command{OpID: "legacy-import-other-actor", Action: "importLegacy", LegacyImport: payload})
	if err != nil || otherActorImport.Receipt.SessionID == first.Receipt.SessionID {
		t.Fatalf("legacy identity was not actor-scoped: receipt=%+v err=%v", otherActorImport.Receipt, err)
	}
}

func TestLegacyImportBoundsAndValidation(t *testing.T) {
	base, p := newTestService(t, nil)
	valid := &LegacyImport{SourceID: "source", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}
	for name, payload := range map[string]*LegacyImport{
		"nil":               nil,
		"unsafe source":     {SourceID: "bad source", Scope: valid.Scope},
		"overlong title":    {SourceID: "long-title", Title: strings.Repeat("x", 257), Scope: valid.Scope},
		"bad scope":         {SourceID: "bad-scope", Scope: Scope{EnvironmentID: "bad env", IdentityDomain: "tenant-a"}},
		"too many entries":  {SourceID: "many", Scope: valid.Scope, Entries: make([]LegacyEntry, MaxSnapshotEntries+1)},
		"too many messages": {SourceID: "many-messages", Scope: valid.Scope, Messages: make([]LegacyChatMessage, 1001)},
		"transaction write cap": {SourceID: "write-cap", Scope: valid.Scope, Messages: func() []LegacyChatMessage {
			values := make([]LegacyChatMessage, 495)
			for i := range values {
				values[i] = LegacyChatMessage{ID: fmt.Sprintf("message-%d", i), Role: "user", Text: "x"}
			}
			return values
		}()},
		"duplicate entry":      {SourceID: "dupe-entry", Scope: valid.Scope, Entries: []LegacyEntry{{ID: "e", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}, {ID: "e", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}}},
		"duplicate alias":      {SourceID: "dupe-alias", Scope: valid.Scope, Entries: []LegacyEntry{{ID: "e1", Alias: "Same", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}, {ID: "e2", Alias: " same ", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}}},
		"invalid subject":      {SourceID: "bad-subject", Scope: valid.Scope, Entries: []LegacyEntry{{ID: "e", Subject: Subject{ConceptID: "C"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}}},
		"invalid value":        {SourceID: "bad-value", Scope: valid.Scope, Entries: []LegacyEntry{{ID: "e", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 2, Kind: "scalar"}}}},
		"bad message role":     {SourceID: "bad-role", Scope: valid.Scope, Messages: []LegacyChatMessage{{ID: "m", Role: "tool", Text: "untrusted"}}},
		"duplicate message ID": {SourceID: "dupe-message", Scope: valid.Scope, Messages: []LegacyChatMessage{{ID: "m", Role: "user", Text: "one"}, {ID: "m", Role: "assistant", Text: "two"}}},
		"oversize message":     {SourceID: "oversize-message", Scope: valid.Scope, Messages: []LegacyChatMessage{{ID: "m", Role: "user", Text: strings.Repeat("x", 100001)}}},
		"oversize payload": {SourceID: "oversize-payload", Scope: valid.Scope, Messages: func() []LegacyChatMessage {
			values := make([]LegacyChatMessage, 45)
			for i := range values {
				values[i] = LegacyChatMessage{ID: fmt.Sprintf("payload-%d", i), Role: "user", Text: strings.Repeat("x", 100000)}
			}
			return values
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := base.Execute(context.Background(), p, Command{OpID: "invalid-" + name, Action: "importLegacy", LegacyImport: payload}); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid legacy payload accepted: %v", err)
			}
		})
	}
	if _, err := base.Execute(context.Background(), p, Command{OpID: "missing-envelope", Action: "importLegacy"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing payload accepted: %v", err)
	}
	if _, err := base.Execute(context.Background(), p, Command{OpID: "unexpected-session", Action: "importLegacy", SessionID: "caller-selected", LegacyImport: valid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("caller-selected target ID accepted: %v", err)
	}
	if _, err := base.Execute(context.Background(), p, Command{OpID: "unexpected-scope", Action: "importLegacy", Scope: &valid.Scope, LegacyImport: valid}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate external scope accepted: %v", err)
	}
}

func TestBoundedReadersRejectCorruptLegacyHistoryAndSnapshotCaps(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	if legacyHash([]string{"a", "bc"}) == legacyHash([]string{"ab", "c"}) {
		t.Fatal("length-prefixed legacy identity hash collided across boundaries")
	}
	badDigest := Command{OpID: "bad-digest", Action: "pinInput", SessionID: "session", DefinitionDocument: json.RawMessage("{"), ResolvedParameters: map[string]apicontract.TypedValue{}}
	if _, err := boundDigest(p, badDigest); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unencodable command digest = %v", err)
	}
	if _, err := base.ListSessions(ctx, p, "", 101); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid list limit = %v", err)
	}

	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		session := Session{SchemaVersion: 1, ID: "too-many-bags", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, State: "active", Version: 1, BagIDs: make([]string, 65)}
		for i := range session.BagIDs {
			session.BagIDs[i] = fmt.Sprintf("bag-%d", i)
		}
		session.ContextSetID = "too-many-set"
		return tx.Save(ctx, Sessions, session.ID, session, 0, p.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(ctx, p, "too-many-bags"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot over bag count cap = %v", err)
	}

	created := createTestSession(t, base, p, "revision-page", "revision-bag")
	put := Command{OpID: "revision-put", Action: "putEntry", SessionID: "revision-page", ExpectedVersion: created.SessionVersion, BagID: "revision-bag", ExpectedBagVersion: 1, EntryID: "revision-entry", RevisionID: "revision-one", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		return tx.Save(ctx, Revisions, "revision-corrupt", EntryRevision{ID: "revision-corrupt", EntryID: "revision-entry", BagID: "different-bag"}, 0, p.Actor, "revision-page")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.ListEntryRevisions(ctx, p, "revision-page", "revision-entry", "", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("revision history corruption = %v", err)
	}
	if _, err := base.ReadEntryRevision(ctx, p, "revision-page", "revision-entry", "missing-revision"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing historical revision = %v", err)
	}
	if _, err := base.ReadEntryRevision(ctx, p, "revision-page", "revision-entry", "revision-corrupt"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign historical revision = %v", err)
	}
	_ = putReceipt
}

func TestSnapshotReaderPropagatesCorruptionAndEntryLimit(t *testing.T) {
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "snapshot-edges", "snapshot-bag")
	put := Command{OpID: "snapshot-entry", Action: "putEntry", SessionID: "snapshot-edges", ExpectedVersion: created.SessionVersion, BagID: "snapshot-bag", ExpectedBagVersion: 1, EntryID: "snapshot-entry", RevisionID: "snapshot-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	executeOK(t, base, p, put)
	for name, collection := range map[string]string{"context set": ContextSets, "bag": Bags, "entry": Entries, "revision": Revisions} {
		t.Run(name, func(t *testing.T) {
			id := map[string]string{"context set": "cs_" + opKey(p, "create-snapshot-edges"), "bag": "snapshot-bag", "entry": "snapshot-entry", "revision": "snapshot-revision"}[name]
			fault := &interceptedRepository{Repository: base.repo, failLoad: func(c, key string) error {
				if c == collection && key == id {
					return ErrUnavailable
				}
				return nil
			}}
			if _, err := interceptService(t, base, p, fault).GetSnapshot(context.Background(), p, "snapshot-edges"); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("snapshot %s load = %v", name, err)
			}
		})
	}
	var session Session
	if err := base.repo.View(context.Background(), func(r Reader) error {
		_, e := r.Load(context.Background(), Sessions, "snapshot-edges", &session)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var bag Bag
	if err := base.repo.View(context.Background(), func(r Reader) error { _, e := r.Load(context.Background(), Bags, "snapshot-bag", &bag); return e }); err != nil {
		t.Fatal(err)
	}
	bag.EntryIDs = make([]string, MaxSnapshotEntries+1)
	for i := range bag.EntryIDs {
		bag.EntryIDs[i] = "snapshot-entry"
	}
	if err := base.repo.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Bags, bag.ID, bag, bag.Version, p.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.GetSnapshot(context.Background(), p, "snapshot-edges"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot entry count cap = %v", err)
	}
}

func TestLegacyImportRejectsSnapshotExpansionBeforeAnyWrite(t *testing.T) {
	base, p := newTestService(t, nil)
	payload := &LegacyImport{SourceID: "snapshot-too-large", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Entries: make([]LegacyEntry, 4)}
	for i := range payload.Entries {
		payload.Entries[i] = LegacyEntry{ID: fmt.Sprintf("large-%d", i), Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: strings.Repeat("m", 600000)}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	}
	if _, err := base.Execute(context.Background(), p, Command{OpID: "large-legacy", Action: "importLegacy", LegacyImport: payload}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("snapshot-expanding import accepted: %v", err)
	}
	if _, err := base.GetSnapshot(context.Background(), p, "legacy_s_"+mustLegacyIdentity(t, p, payload)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed import left session behind: %v", err)
	}
}

func TestExecuteVersionAndValidationFailuresDoNotMutate(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	created := createTestSession(t, base, p, "execute-edges", "execute-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	invalidCommands := []Command{
		{OpID: "bad-bag-id", Action: "createBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "bad id", Scope: &scope},
		{OpID: "bad-bag-scope", Action: "createBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "valid-bag"},
		{OpID: "long-bag-title", Action: "createBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "valid-bag", Scope: &scope, Title: strings.Repeat("x", 257)},
		{OpID: "blank-session-title", Action: "renameSession", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, Title: "  "},
		{OpID: "long-session-title", Action: "renameSession", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, Title: strings.Repeat("x", 257)},
		{OpID: "blank-bag-title", Action: "renameBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, Title: " "},
		{OpID: "missing-enabled", Action: "applyContext", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, ExpectedContextSetID: "cs_" + opKey(p, "create-execute-edges"), Scope: &scope},
		{OpID: "bad-entry-id", Action: "putEntry", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, EntryID: "bad id", RevisionID: "revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}},
		{OpID: "missing-subject-field", Action: "putEntry", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Subject: &Subject{ConceptID: "C", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}},
		{OpID: "bad-alias", Action: "putEntry", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Alias: strings.Repeat("a", 129), Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}},
		{OpID: "bad-value", Action: "putEntry", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 2, Kind: "scalar"}},
	}
	for _, command := range invalidCommands {
		if _, err := base.Execute(ctx, p, command); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s = %v", command.OpID, err)
		}
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "wrong-slot-version", Action: "clearBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, ExpectedSlotVersion: 1, Scope: &scope}); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong applied slot version = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "stale-bag-title", Action: "renameBag", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 2, Title: "Title"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale bag rename = %v", err)
	}

	put := Command{OpID: "execute-valid-entry", Action: "putEntry", SessionID: "execute-edges", ExpectedVersion: created.SessionVersion, BagID: "execute-bag", ExpectedBagVersion: 1, EntryID: "execute-entry", RevisionID: "execute-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	putReceipt := executeOK(t, base, p, put)
	stale := put
	stale.OpID, stale.ExpectedVersion, stale.ExpectedBagVersion, stale.ExpectedEntryVersion, stale.RevisionID = "stale-entry", putReceipt.SessionVersion, 1, 0, "new-revision"
	if _, err := base.Execute(ctx, p, stale); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale bag put = %v", err)
	}
	changedRevision := put
	changedRevision.OpID, changedRevision.ExpectedVersion, changedRevision.ExpectedBagVersion, changedRevision.ExpectedEntryVersion, changedRevision.RevisionID = "stale-entry-version", putReceipt.SessionVersion, 2, 0, "new-revision"
	if _, err := base.Execute(ctx, p, changedRevision); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale entry version = %v", err)
	}
	wrongReference := Command{OpID: "stale-selection", Action: "applyContext", SessionID: "execute-edges", ExpectedVersion: putReceipt.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-execute-edges"), BagID: "execute-bag", ExpectedBagVersion: 2, ExpectedSlotVersion: 0, Enabled: boolPtr(true), Scope: &scope, Selection: []EntryRef{{EntryID: "execute-entry", RevisionID: "stale-revision"}}}
	if _, err := base.Execute(ctx, p, wrongReference); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale applied ref = %v", err)
	}
	wrongContextSet := wrongReference
	wrongContextSet.OpID, wrongContextSet.ExpectedContextSetID, wrongContextSet.ExpectedVersion = "wrong-set", "stale-set", putReceipt.SessionVersion
	if _, err := base.Execute(ctx, p, wrongContextSet); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale context set = %v", err)
	}

	server := p
	server.Executor = true
	check := Command{OpID: "execute-check", Action: "pinInput", SessionID: "execute-edges", ExpectedVersion: putReceipt.SessionVersion, InputSnapshotID: "execute-input", RunID: "execute-run", Kind: "check", DefinitionID: "check", DefinitionRevision: "r1", DefinitionDocument: testDefinition("check"), Scope: &scope}
	checkReceipt := executeOK(t, base, server, check)
	invalidOutcome := Command{OpID: "invalid-server-outcome", Action: "completeRun", SessionID: "execute-edges", ExpectedRunVersion: 1, RunID: "execute-run", RunState: "failed", ErrorCode: "unsafe code"}
	if _, err := base.Execute(ctx, server, invalidOutcome); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid server outcome = %v", err)
	}
	invalidLink := invalidOutcome
	invalidLink.OpID, invalidLink.RunState, invalidLink.ErrorCode, invalidLink.CaseID, invalidLink.DetectiveRunID = "invalid-link", "completed", "", "bad id", "run"
	if _, err := base.Execute(ctx, server, invalidLink); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid evidence link = %v", err)
	}
	clientComplete := invalidOutcome
	clientComplete.OpID, clientComplete.RunState, clientComplete.ErrorCode = "client-complete", "completed", ""
	if _, err := base.Execute(ctx, p, clientComplete); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client completeRun = %v", err)
	}
	_ = checkReceipt
}

func TestExecutePropagatesEachRepositoryWriteFailureAtomically(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	tests := []struct {
		name, action, failCollection string
		prepare                      func(*testing.T, *Service, Principal) Command
		principalExecutor            bool
	}{
		{name: "create session bag", action: "createSession", failCollection: Bags, prepare: func(_ *testing.T, _ *Service, p Principal) Command {
			return Command{OpID: "create-session-write", Action: "createSession", SessionID: "create-write", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "create-write-bag", Scope: &scope}
		}},
		{name: "create session set", action: "createSession", failCollection: ContextSets, prepare: func(_ *testing.T, _ *Service, p Principal) Command {
			return Command{OpID: "create-set-write", Action: "createSession", SessionID: "create-set", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "create-set-bag", Scope: &scope}
		}},
		{name: "create bag", action: "createBag", failCollection: Bags, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "create-bag-write", "create-bag-base")
			return Command{OpID: "create-extra-bag", Action: "createBag", SessionID: "create-bag-write", ExpectedVersion: r.SessionVersion, BagID: "extra-bag", Scope: &scope}
		}},
		{name: "session rename", action: "renameSession", failCollection: Sessions, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "rename-session-write", "rename-session-bag")
			return Command{OpID: "rename-session-write", Action: "renameSession", SessionID: "rename-session-write", ExpectedVersion: r.SessionVersion, Title: "new title"}
		}},
		{name: "bag rename", action: "renameBag", failCollection: Bags, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "rename-bag-write", "rename-bag")
			return Command{OpID: "rename-bag-write-op", Action: "renameBag", SessionID: "rename-bag-write", ExpectedVersion: r.SessionVersion, BagID: "rename-bag", ExpectedBagVersion: 1, Title: "new title"}
		}},
		{name: "revision write", action: "putEntry", failCollection: Revisions, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "revision-write", "revision-bag")
			return validPutCommand(r, "revision-write", "revision-bag")
		}},
		{name: "entry write", action: "putEntry", failCollection: Entries, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "entry-write", "entry-bag")
			return validPutCommand(r, "entry-write", "entry-bag")
		}},
		{name: "bag write", action: "putEntry", failCollection: Bags, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "bag-write", "bag-write-bag")
			return validPutCommand(r, "bag-write", "bag-write-bag")
		}},
		{name: "session write", action: "createBag", failCollection: Sessions, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "session-write", "session-write-bag")
			return Command{OpID: "create-before-session-fail", Action: "createBag", SessionID: "session-write", ExpectedVersion: r.SessionVersion, BagID: "new-bag", Scope: &scope}
		}},
		{name: "audit write", action: "createBag", failCollection: Audit, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "audit-write", "audit-bag")
			return Command{OpID: "audit-write-op", Action: "createBag", SessionID: "audit-write", ExpectedVersion: r.SessionVersion, BagID: "new-bag", Scope: &scope}
		}},
		{name: "receipt write", action: "createBag", failCollection: Operations, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "receipt-write", "receipt-bag")
			return Command{OpID: "receipt-write-op", Action: "createBag", SessionID: "receipt-write", ExpectedVersion: r.SessionVersion, BagID: "new-bag", Scope: &scope}
		}},
		{name: "input write", action: "pinInput", failCollection: Inputs, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "input-write", "input-bag")
			return validPinCommand(r, "input-write", "input-write-run", "input-write-input", scope, "query")
		}},
		{name: "run write", action: "pinInput", failCollection: Runs, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "run-write", "run-bag")
			return validPinCommand(r, "run-write", "run-write-run", "run-write-input", scope, "query")
		}},
		{name: "chat message write", action: "pinInput", failCollection: Messages, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "message-write", "message-bag")
			c := validPinCommand(r, "message-write", "message-write-run", "message-write-input", scope, "chat")
			c.Text = "question"
			return c
		}},
		{name: "run update", action: "completeRun", failCollection: Runs, principalExecutor: true, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "run-update", "run-update-bag")
			p.Executor = true
			executeOK(t, s, p, validPinCommand(r, "run-update", "server-run", "server-input", scope, "check"))
			return Command{OpID: "server-complete-fail", Action: "completeRun", SessionID: "run-update", RunID: "server-run", ExpectedRunVersion: 1, RunState: "completed"}
		}},
		{name: "apply context set write", action: "applyContext", failCollection: ContextSets, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "apply-set-write", "apply-set-bag")
			return Command{OpID: "apply-set-write-op", Action: "applyContext", SessionID: "apply-set-write", ExpectedVersion: r.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-apply-set-write"), BagID: "apply-set-bag", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(true)}
		}},
		{name: "clear context set write", action: "clearBag", failCollection: ContextSets, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "clear-set-write", "clear-set-bag")
			applied := executeOK(t, s, p, Command{OpID: "clear-set-apply", Action: "applyContext", SessionID: "clear-set-write", ExpectedVersion: r.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-clear-set-write"), BagID: "clear-set-bag", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(true)})
			return Command{OpID: "clear-set-write-op", Action: "clearBag", SessionID: "clear-set-write", ExpectedVersion: applied.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "clear-set-apply"), BagID: "clear-set-bag", ExpectedBagVersion: 1, ExpectedSlotVersion: 1, Scope: &scope}
		}},
		{name: "clear bag write", action: "clearBag", failCollection: Bags, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "clear-bag-write", "clear-bag-write-id")
			applied := executeOK(t, s, p, Command{OpID: "clear-bag-apply", Action: "applyContext", SessionID: r.SessionID, ExpectedVersion: r.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-clear-bag-write"), BagID: "clear-bag-write-id", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(true)})
			return Command{OpID: "clear-bag-write-op", Action: "clearBag", SessionID: r.SessionID, ExpectedVersion: applied.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "clear-bag-apply"), BagID: "clear-bag-write-id", ExpectedBagVersion: 1, ExpectedSlotVersion: 1, Scope: &scope}
		}},
		{name: "client assistant message write", action: "reportClientRun", failCollection: Messages, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "client-message-write", "client-message-bag")
			executeOK(t, s, p, validPinCommand(r, "client-message-write", "client-message-run", "client-message-input", scope, "chat"))
			return Command{OpID: "client-message-fail", Action: "reportClientRun", SessionID: "client-message-write", RunID: "client-message-run", Kind: "chat", ExpectedRunVersion: 1, RunState: "completed", Text: "answer"}
		}},
		{name: "outbox write", action: "completeRun", failCollection: Outbox, principalExecutor: true, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "outbox-write", "outbox-bag")
			p.Executor = true
			executeOK(t, s, p, validPinCommand(r, "outbox-write", "outbox-run", "outbox-input", scope, "check"))
			return Command{OpID: "outbox-complete-fail", Action: "completeRun", SessionID: "outbox-write", RunID: "outbox-run", ExpectedRunVersion: 1, RunState: "completed", CaseID: "case", DetectiveRunID: "detective-run"}
		}},
		{name: "execution plan write", action: "recordRunExecution", failCollection: Runs, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "plan-write", "plan-bag")
			pinned := executeOK(t, s, p, validPinCommand(r, "plan-write", "plan-run", "plan-input", scope, "chat"))
			return Command{OpID: "plan-write-op", Action: "recordRunExecution", SessionID: "plan-write", ExpectedVersion: pinned.SessionVersion, RunID: "plan-run", ExpectedRunVersion: 1, ExecutionDocument: testQueryDefinition("select 1")}
		}},
		{name: "dashboard report write", action: "reportDashboard", failCollection: Runs, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "dashboard-write", "dashboard-bag")
			executeOK(t, s, p, validPinCommand(r, "dashboard-write", "dashboard-run", "dashboard-input", scope, "dashboard"))
			return Command{OpID: "dashboard-report-fail", Action: "reportDashboard", SessionID: "dashboard-write", RunID: "dashboard-run", ExpectedRunVersion: 1, RunState: "completed"}
		}},
		{name: "client report write", action: "reportClientRun", failCollection: Runs, prepare: func(t *testing.T, s *Service, p Principal) Command {
			r := createTestSession(t, s, p, "client-report-write", "client-report-bag")
			executeOK(t, s, p, validPinCommand(r, "client-report-write", "client-report-run", "client-report-input", scope, "query"))
			return Command{OpID: "client-report-fail", Action: "reportClientRun", SessionID: "client-report-write", RunID: "client-report-run", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, p := newTestService(t, nil)
			command := test.prepare(t, base, p)
			if test.principalExecutor {
				p.Executor = true
			}
			fault := &interceptedRepository{Repository: base.repo, failSave: func(collection, _ string) error {
				if collection == test.failCollection {
					return errors.New("forced write error")
				}
				return nil
			}}
			_, err := interceptService(t, base, p, fault).Execute(ctx, p, command)
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("expected failure at %s, got %v", test.failCollection, err)
			}
		})
	}
}

func validPutCommand(sessionReceipt Receipt, sessionID, bagID string) Command {
	return Command{OpID: "put-" + sessionID, Action: "putEntry", SessionID: sessionID, ExpectedVersion: sessionReceipt.SessionVersion, BagID: bagID, ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("value"))}}
}

func validPinCommand(sessionReceipt Receipt, sessionID, runID, inputID string, scope Scope, kind string) Command {
	return Command{OpID: "pin-" + runID, Action: "pinInput", SessionID: sessionID, ExpectedVersion: sessionReceipt.SessionVersion, InputSnapshotID: inputID, RunID: runID, Kind: kind, DefinitionID: "definition", DefinitionRevision: "r1", DefinitionDocument: testDefinition(kind), Scope: &scope}
}

func TestLegacyImportRepositoryFailuresRollbackEveryRecord(t *testing.T) {
	ctx := context.Background()
	payload := &LegacyImport{SourceID: "faulted-source", Title: "Imported", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Entries: []LegacyEntry{{ID: "e", Subject: Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("private"))}}}, Messages: []LegacyChatMessage{{ID: "m", Role: "user", Text: "private question"}}}
	for _, collection := range []string{Revisions, Entries, Messages, Bags, ContextSets, LegacyImports, Sessions, Audit, Operations} {
		t.Run(collection, func(t *testing.T) {
			base, p := newTestService(t, nil)
			fault := &interceptedRepository{Repository: base.repo, failSave: func(c, _ string) error {
				if c == collection {
					return errors.New("storage failure")
				}
				return nil
			}}
			_, err := interceptService(t, base, p, fault).Execute(ctx, p, Command{OpID: "legacy-fail", Action: "importLegacy", LegacyImport: payload})
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("failure at %s returned %v", collection, err)
			}
			prepared, _, _, prepareErr := prepareLegacyImport(p, payload)
			if prepareErr != nil {
				t.Fatal(prepareErr)
			}
			if _, err = base.GetSnapshot(ctx, p, prepared.SessionID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("partial import left session state: %v", err)
			}
		})
	}
}

func TestExecuteRejectsAuthorizationAndRepositoryReadFailures(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "execute-read-errors", "execute-read-bag")
	if _, err := base.Execute(ctx, p, Command{OpID: "import-field-elsewhere", Action: "renameSession", SessionID: "execute-read-errors", ExpectedVersion: created.SessionVersion, Title: "new", LegacyImport: &LegacyImport{SourceID: "s", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("import payload on other action = %v", err)
	}
	denied := interceptService(t, base, p, nil)
	denied.authorize = func(context.Context, Access) error { return ErrForbidden }
	if _, err := denied.Execute(ctx, p, Command{OpID: "auth-denied", Action: "renameSession", SessionID: "execute-read-errors", ExpectedVersion: created.SessionVersion, Title: "new"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("execute authorization error = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "digest-invalid", Action: "pinInput", SessionID: "execute-read-errors", InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "query", DefinitionRevision: "r1", DefinitionDocument: json.RawMessage("{"), Scope: &Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid digest payload = %v", err)
	}
	failOperations := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == Operations {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, base, p, failOperations).Execute(ctx, p, Command{OpID: "operation-read-fail", Action: "renameSession", SessionID: "execute-read-errors", ExpectedVersion: created.SessionVersion, Title: "new"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("operation lookup failure = %v", err)
	}
	failSession := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == Sessions && id == "execute-read-errors" {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, base, p, failSession).Execute(ctx, p, Command{OpID: "session-read-fail", Action: "renameSession", SessionID: "execute-read-errors", ExpectedVersion: created.SessionVersion, Title: "new"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("session lookup failure = %v", err)
	}

	payload := &LegacyImport{SourceID: "manifest-read-fail", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}
	manifestFault := &interceptedRepository{Repository: base.repo, failLoad: func(collection, id string) error {
		if collection == LegacyImports {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, base, p, manifestFault).Execute(ctx, p, Command{OpID: "manifest-read-fail", Action: "importLegacy", LegacyImport: payload}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("manifest lookup failure = %v", err)
	}
}

func TestExecutePropagatesRepositoryReadFailuresAcrossMutationPaths(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	tests := []struct {
		name, failCollection string
		prepare              func(*testing.T, *Service, Principal) (Command, Principal)
	}{
		{"rename bag lookup", Bags, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-rename-bag", "lookup-rename-bag-id")
			return Command{OpID: "lookup-rename-bag-op", Action: "renameBag", SessionID: r.SessionID, ExpectedVersion: r.SessionVersion, BagID: "lookup-rename-bag-id", ExpectedBagVersion: 1, Title: "renamed"}, p
		}},
		{"put entry lookup", Entries, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-put-entry", "lookup-put-bag")
			return validPutCommand(r, r.SessionID, "lookup-put-bag"), p
		}},
		{"put bag lookup", Bags, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-put-bag-record", "lookup-put-bag-record-id")
			return validPutCommand(r, r.SessionID, "lookup-put-bag-record-id"), p
		}},
		{"put alias sibling lookup", Entries, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-alias-entry", "lookup-alias-bag")
			first := validPutCommand(r, r.SessionID, "lookup-alias-bag")
			first.EntryID, first.RevisionID, first.Alias = "existing-alias-entry", "existing-alias-revision", "first"
			executeOK(t, s, p, first)
			next := validPutCommand(r, r.SessionID, "lookup-alias-bag")
			next.OpID, next.ExpectedVersion, next.ExpectedBagVersion, next.EntryID, next.RevisionID, next.Alias = "alias-sibling-op", 2, 2, "new-alias-entry", "new-alias-revision", "second"
			return next, p
		}},
		{"apply set lookup", ContextSets, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-apply-set", "lookup-apply-bag")
			return Command{OpID: "lookup-apply-set-op", Action: "applyContext", SessionID: r.SessionID, ExpectedVersion: r.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-lookup-apply-set"), BagID: "lookup-apply-bag", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(true)}, p
		}},
		{"apply bag lookup", Bags, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-apply-bag", "lookup-apply-bag-id")
			return Command{OpID: "lookup-apply-bag-op", Action: "applyContext", SessionID: r.SessionID, ExpectedVersion: r.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-lookup-apply-bag"), BagID: "lookup-apply-bag-id", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(true)}, p
		}},
		{"apply stale revision", Revisions, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-apply-revision", "lookup-apply-revision-bag")
			put := validPutCommand(r, r.SessionID, "lookup-apply-revision-bag")
			put.EntryID, put.RevisionID = "lookup-apply-revision-entry", "lookup-apply-revision-id"
			written := executeOK(t, s, p, put)
			return Command{OpID: "lookup-apply-revision-op", Action: "applyContext", SessionID: r.SessionID, ExpectedVersion: written.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-lookup-apply-revision"), BagID: "lookup-apply-revision-bag", ExpectedBagVersion: 2, Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: put.EntryID, RevisionID: put.RevisionID}}}, p
		}},
		{"complete run lookup", Runs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-complete-run", "lookup-complete-bag")
			p.Executor = true
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-complete-run-id", "lookup-complete-input", scope, "check"))
			return Command{OpID: "lookup-complete-op", Action: "completeRun", SessionID: r.SessionID, RunID: "lookup-complete-run-id", ExpectedRunVersion: 1, RunState: "completed"}, p
		}},
		{"complete input lookup", Inputs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-complete-input", "lookup-complete-input-bag")
			p.Executor = true
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-complete-input-run", "lookup-complete-input-id", scope, "check"))
			return Command{OpID: "lookup-complete-input-op", Action: "completeRun", SessionID: r.SessionID, RunID: "lookup-complete-input-run", ExpectedRunVersion: 1, RunState: "completed"}, p
		}},
		{"execution plan lookup", Runs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-plan-run", "lookup-plan-bag")
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-plan-id", "lookup-plan-input", scope, "chat"))
			return Command{OpID: "lookup-plan-op", Action: "recordRunExecution", SessionID: r.SessionID, ExpectedVersion: 2, RunID: "lookup-plan-id", ExpectedRunVersion: 1, ExecutionDocument: testQueryDefinition("select 1")}, p
		}},
		{"execution input lookup", Inputs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-plan-input", "lookup-plan-input-bag")
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-plan-input-run", "lookup-plan-input-id", scope, "chat"))
			return Command{OpID: "lookup-plan-input-op", Action: "recordRunExecution", SessionID: r.SessionID, ExpectedVersion: 2, RunID: "lookup-plan-input-run", ExpectedRunVersion: 1, ExecutionDocument: testQueryDefinition("select 1")}, p
		}},
		{"client completion lookup", Runs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-report-run", "lookup-report-bag")
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-report-run-id", "lookup-report-input", scope, "query"))
			return Command{OpID: "lookup-report-op", Action: "reportClientRun", SessionID: r.SessionID, RunID: "lookup-report-run-id", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}, p
		}},
		{"dashboard input lookup", Inputs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-dashboard-input", "lookup-dashboard-bag")
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-dashboard-run", "lookup-dashboard-input-id", scope, "dashboard"))
			return Command{OpID: "lookup-dashboard-report", Action: "reportDashboard", SessionID: r.SessionID, RunID: "lookup-dashboard-run", ExpectedRunVersion: 1, RunState: "completed"}, p
		}},
		{"client result input lookup", Inputs, func(t *testing.T, s *Service, p Principal) (Command, Principal) {
			r := createTestSession(t, s, p, "lookup-client-input", "lookup-client-bag")
			executeOK(t, s, p, validPinCommand(r, r.SessionID, "lookup-client-input-run", "lookup-client-input-id", scope, "query"))
			return Command{OpID: "lookup-client-input-report", Action: "reportClientRun", SessionID: r.SessionID, RunID: "lookup-client-input-run", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}, p
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, p := newTestService(t, nil)
			command, principal := test.prepare(t, base, p)
			fault := &interceptedRepository{Repository: base.repo, failLoad: func(collection, _ string) error {
				if collection == test.failCollection {
					return ErrUnavailable
				}
				return nil
			}}
			if _, err := interceptService(t, base, principal, fault).Execute(ctx, principal, command); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("load failure at %s = %v", test.failCollection, err)
			}
		})
	}
}

func TestExecuteReplayReauthorizesAndPropagatesLateReadFailures(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	created := createTestSession(t, base, p, "replay-read-errors", "replay-read-bag")
	command := Command{OpID: "replay-read-op", Action: "renameSession", SessionID: created.SessionID, ExpectedVersion: created.SessionVersion, Title: "Renamed"}
	if _, err := base.Execute(ctx, p, command); err != nil {
		t.Fatal(err)
	}
	for name, fault := range map[string]func(string, string) error{
		"session load": func(c, id string) error {
			if c == Sessions && id == created.SessionID {
				return ErrUnavailable
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			faulty := interceptService(t, base, p, &interceptedRepository{Repository: base.repo, failLoad: fault})
			if _, err := faulty.Execute(ctx, p, command); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("replay load = %v", err)
			}
		})
	}
	denied := interceptService(t, base, p, nil)
	authorizations := 0
	denied.authorize = func(context.Context, Access) error {
		authorizations++
		if authorizations == 2 {
			return ErrForbidden
		} // outer request passes; replay recheck is revoked
		return nil
	}
	if _, err := denied.Execute(ctx, p, command); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay fresh authorization = %v", err)
	}

	legacyBase, legacyPrincipal := newTestService(t, nil)
	legacy := &LegacyImport{SourceID: "late-snapshot-error", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}}
	legacyFault := &interceptedRepository{Repository: legacyBase.repo, failLoad: func(c, _ string) error {
		if c == Bags {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, legacyBase, legacyPrincipal, legacyFault).Execute(ctx, legacyPrincipal, Command{OpID: "late-import-error", Action: "importLegacy", LegacyImport: legacy}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("legacy snapshot read failure = %v", err)
	}
}

func TestExecuteConditionalGuardsRejectPendingDeleteAndMalformedOutcomes(t *testing.T) {
	ctx := context.Background()
	base, p := newTestService(t, nil)
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	created := createTestSession(t, base, p, "conditional-guards", "conditional-guards-bag")
	pin := validPinCommand(created, created.SessionID, "pending-delete-run", "pending-delete-input", scope, "chat")
	pin.Text = "question"
	executeOK(t, base, p, pin)
	if _, err := base.Execute(ctx, p, Command{OpID: "delete-pending", Action: "transitionSession", SessionID: created.SessionID, ExpectedVersion: 2, Transition: "delete"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("delete with accepted run = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "report-blank-chat", Action: "reportClientRun", SessionID: created.SessionID, RunID: "pending-delete-run", Kind: "chat", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("completed blank chat = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "report-chat-too-long", Action: "reportClientRun", SessionID: created.SessionID, RunID: "pending-delete-run", Kind: "chat", ExpectedRunVersion: 1, RunState: "failed", ErrorCode: "EXEC_FAILED", Text: strings.Repeat("x", 100001)}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized chat text = %v", err)
	}
	queryPin := validPinCommand(Receipt{SessionID: created.SessionID, SessionVersion: 2}, created.SessionID, "pending-delete-query-run", "pending-delete-query-input", scope, "query")
	executeOK(t, base, p, queryPin)
	if _, err := base.Execute(ctx, p, Command{OpID: "report-query-text", Action: "reportClientRun", SessionID: created.SessionID, RunID: "pending-delete-query-run", Kind: "query", ExpectedRunVersion: 1, RunState: "failed", ErrorCode: "EXEC_FAILED", Text: "unexpected"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("query result with chat text = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "report-wrong-kind", Action: "reportClientRun", SessionID: created.SessionID, RunID: "pending-delete-run", Kind: "dashboard", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("run kind mismatch = %v", err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "report-stale-version", Action: "reportClientRun", SessionID: created.SessionID, RunID: "pending-delete-run", Kind: "query", ExpectedRunVersion: 3, RunState: "completed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale run version = %v", err)
	}
}

func TestExecuteCoversReplayIntegrityAndLateMutationGuards(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	base, p := newTestService(t, nil)
	create := Command{OpID: "replay-create", Action: "createSession", SessionID: "replay-create-session", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "replay-create-bag", Scope: &scope}
	if _, err := base.Execute(ctx, p, create); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Execute(ctx, p, create); err != nil {
		t.Fatalf("create replay with bag recheck: %v", err)
	}

	created := createTestSession(t, base, p, "integrity-entry", "integrity-bag")
	put := validPutCommand(created, created.SessionID, "integrity-bag")
	put.EntryID, put.RevisionID = "integrity-entry-id", "integrity-rev"
	executeOK(t, base, p, put)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		var entry Entry
		if _, e := tx.Load(ctx, Entries, put.EntryID, &entry); e != nil {
			return e
		}
		entry.BagID = "other-bag"
		return tx.Save(ctx, Entries, entry.ID, entry, entry.Version, p.Actor, created.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	next := put
	next.OpID, next.ExpectedVersion, next.ExpectedBagVersion, next.ExpectedEntryVersion, next.RevisionID = "integrity-entry-update", 2, 2, 1, "integrity-rev-2"
	if _, err := base.Execute(ctx, p, next); !errors.Is(err, ErrForbidden) {
		t.Fatalf("corrupt entry owner = %v", err)
	}

	aliasSession := createTestSession(t, base, p, "integrity-alias", "integrity-alias-bag")
	first := validPutCommand(aliasSession, aliasSession.SessionID, "integrity-alias-bag")
	first.EntryID, first.RevisionID, first.Alias = "integrity-alias-entry", "integrity-alias-rev", "First"
	executeOK(t, base, p, first)
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		var entry Entry
		if _, e := tx.Load(ctx, Entries, first.EntryID, &entry); e != nil {
			return e
		}
		entry.SessionID = "other-session"
		return tx.Save(ctx, Entries, entry.ID, entry, entry.Version, p.Actor, aliasSession.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	second := validPutCommand(Receipt{SessionID: aliasSession.SessionID, SessionVersion: 2}, aliasSession.SessionID, "integrity-alias-bag")
	second.OpID, second.ExpectedBagVersion, second.EntryID, second.RevisionID, second.Alias = "integrity-alias-second", 2, "integrity-alias-entry-2", "integrity-alias-rev-2", "Second"
	if _, err := base.Execute(ctx, p, second); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("corrupt alias sibling = %v", err)
	}

	capSession := createTestSession(t, base, p, "entry-cap", "entry-cap-bag")
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		var bag Bag
		if _, e := tx.Load(ctx, Bags, "entry-cap-bag", &bag); e != nil {
			return e
		}
		bag.EntryIDs = make([]string, 1000)
		for i := range bag.EntryIDs {
			bag.EntryIDs[i] = fmt.Sprintf("existing-%d", i)
		}
		bag.Version++
		return tx.Save(ctx, Bags, bag.ID, bag, bag.Version-1, p.Actor, capSession.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	capPut := validPutCommand(capSession, capSession.SessionID, "entry-cap-bag")
	capPut.OpID, capPut.ExpectedBagVersion, capPut.EntryID, capPut.RevisionID = "entry-cap-op", 2, "entry-cap-new", "entry-cap-new-rev"
	if _, err := base.Execute(ctx, p, capPut); !errors.Is(err, ErrInvalid) {
		t.Fatalf("entry cap = %v", err)
	}

	applySession := createTestSession(t, base, p, "stale-apply", "stale-apply-bag")
	staleApply := Command{OpID: "stale-apply-op", Action: "applyContext", SessionID: applySession.SessionID, ExpectedVersion: applySession.SessionVersion, ExpectedContextSetID: "stale-context-set", BagID: "stale-apply-bag", ExpectedBagVersion: 1, Scope: &scope, Enabled: boolPtr(false)}
	if _, err := base.Execute(ctx, p, staleApply); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale context set = %v", err)
	}

	authorized := interceptService(t, base, p, nil)
	authorized.authorize = func(_ context.Context, a Access) error {
		if a.Action == "pinInput" && a.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := authorized.Execute(ctx, p, validPinCommand(applySession, applySession.SessionID, "denied-pin", "denied-pin-input", scope, "query")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pin scoped authorization = %v", err)
	}
}

func TestRunCommandsEnforceFreshAuthorityScopeAndPendingCounters(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	for _, test := range []struct {
		name   string
		kind   string
		finish func(*Service, Principal, Receipt) error
	}{
		{"server check completion", "check", func(s *Service, p Principal, r Receipt) error {
			p.Executor = true
			_, err := s.Execute(ctx, p, Command{OpID: "complete-guard", Action: "completeRun", SessionID: r.SessionID, RunID: "run-guard", ExpectedRunVersion: 1, RunState: "completed", ErrorCode: "INVALID"})
			return err
		}},
		{"server completion cannot seize client run", "query", func(s *Service, p Principal, r Receipt) error {
			p.Executor = true
			_, err := s.Execute(ctx, p, Command{OpID: "complete-wrong-authority", Action: "completeRun", SessionID: r.SessionID, RunID: "run-guard", ExpectedRunVersion: 1, RunState: "completed"})
			return err
		}},
		{"dashboard outcome validation", "dashboard", func(s *Service, p Principal, r Receipt) error {
			_, err := s.Execute(ctx, p, Command{OpID: "dashboard-invalid-outcome", Action: "reportDashboard", SessionID: r.SessionID, RunID: "run-guard", ExpectedRunVersion: 1, RunState: "completed", ResultRefs: []string{"unsafe ref"}})
			return err
		}},
		{"client report outcome validation", "query", func(s *Service, p Principal, r Receipt) error {
			_, err := s.Execute(ctx, p, Command{OpID: "client-invalid-outcome", Action: "reportClientRun", SessionID: r.SessionID, RunID: "run-guard", Kind: "query", ExpectedRunVersion: 1, RunState: "completed", ResultRefs: []string{"unsafe ref"}})
			return err
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base, p := newTestService(t, nil)
			r := createTestSession(t, base, p, "run-guard-session", "run-guard-bag")
			if test.kind == "check" {
				p.Executor = true
			}
			pin := validPinCommand(r, r.SessionID, "run-guard", "run-guard-input", scope, test.kind)
			if test.kind == "chat" {
				pin.Text = "question"
			}
			executeOK(t, base, p, pin)
			want := ErrInvalid
			if test.name == "server completion cannot seize client run" {
				want = ErrForbidden
			}
			if err := test.finish(base, p, r); !errors.Is(err, want) {
				t.Fatalf("run operation = %v, want %v", err, want)
			}
		})
	}

	// The retained session counter is part of the deletion guard. Corruption that
	// would otherwise underflow must abort the same terminal transaction.
	base, p := newTestService(t, nil)
	r := createTestSession(t, base, p, "pending-counter", "pending-counter-bag")
	executeOK(t, base, p, validPinCommand(r, r.SessionID, "pending-counter-run", "pending-counter-input", scope, "query"))
	if err := base.repo.Transact(ctx, func(tx Transaction) error {
		var session Session
		if _, e := tx.Load(ctx, Sessions, r.SessionID, &session); e != nil {
			return e
		}
		session.PendingRunCount = 0
		session.Version++
		return tx.Save(ctx, Sessions, session.ID, session, session.Version-1, p.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := base.Execute(ctx, p, Command{OpID: "counter-underflow-report", Action: "reportClientRun", SessionID: r.SessionID, RunID: "pending-counter-run", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pending counter underflow = %v", err)
	}
}

func TestRemainingExecutionFailureBranches(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	newRun := func(t *testing.T, kind string, executor bool) (*Service, Principal, Receipt) {
		t.Helper()
		svc, principal := newTestService(t, nil)
		principal.Executor = executor
		r := createTestSession(t, svc, principal, "remaining-run-session", "remaining-run-bag")
		pin := validPinCommand(r, r.SessionID, "remaining-run-id", "remaining-run-input", scope, kind)
		if kind == "chat" {
			pin.Text = "question"
		}
		executeOK(t, svc, principal, pin)
		return svc, principal, r
	}
	// A replay performs a second scoped authorization after rechecking its bag.
	base, p := newTestService(t, nil)
	create := Command{OpID: "replay-scope-auth", Action: "createSession", SessionID: "replay-scope-auth-session", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "replay-scope-auth-bag", Scope: &scope}
	executeOK(t, base, p, create)
	replay := interceptService(t, base, p, nil)
	checks := 0
	replay.authorize = func(context.Context, Access) error {
		checks++
		if checks == 4 {
			return ErrForbidden
		}
		return nil
	}
	if _, err := replay.Execute(ctx, p, create); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay scoped auth = %v", err)
	}

	applyBase, applyPrincipal := newTestService(t, nil)
	applyReceipt := createTestSession(t, applyBase, applyPrincipal, "apply-version-guard", "apply-version-bag")
	applyCommand := Command{OpID: "apply-version-mismatch", Action: "applyContext", SessionID: applyReceipt.SessionID, ExpectedVersion: applyReceipt.SessionVersion, ExpectedContextSetID: "cs_" + opKey(applyPrincipal, "create-apply-version-guard"), BagID: "apply-version-bag", ExpectedBagVersion: 99, Scope: &scope, Enabled: boolPtr(true)}
	if _, err := applyBase.Execute(ctx, applyPrincipal, applyCommand); !errors.Is(err, ErrConflict) {
		t.Fatalf("apply bag version conflict = %v", err)
	}
	applyCommand.OpID, applyCommand.ExpectedBagVersion, applyCommand.ExpectedVersion = "apply-session-version-mismatch", 1, 0
	if _, err := applyBase.Execute(ctx, applyPrincipal, applyCommand); !errors.Is(err, ErrConflict) {
		t.Fatalf("apply session version conflict = %v", err)
	}

	// Scoped authorization is rechecked after a command's session-level check.
	pinSvc, pinPrincipal := newTestService(t, nil)
	pinCreated := createTestSession(t, pinSvc, pinPrincipal, "pin-scope-auth", "pin-scope-bag")
	pinChecks := 0
	pinSvc.authorize = func(_ context.Context, access Access) error {
		pinChecks++
		if access.Action == "pinInput" && access.Scope != nil && pinChecks == 3 {
			return ErrForbidden
		}
		return nil
	}
	if _, err := pinSvc.Execute(ctx, pinPrincipal, validPinCommand(pinCreated, pinCreated.SessionID, "pin-scope-run", "pin-scope-input", scope, "query")); !errors.Is(err, ErrForbidden) {
		t.Fatalf("pin scoped authorization = %v", err)
	}

	serverSvc, serverPrincipal, serverReceipt := newRun(t, "check", true)
	serverPrincipal.Executor = true
	if _, err := serverSvc.Execute(ctx, serverPrincipal, Command{OpID: "server-version-conflict", Action: "completeRun", SessionID: serverReceipt.SessionID, RunID: "remaining-run-id", ExpectedRunVersion: 2, RunState: "completed"}); !errors.Is(err, ErrConflict) {
		t.Fatalf("server stale run version = %v", err)
	}
	serverDenied, deniedPrincipal, deniedReceipt := newRun(t, "check", true)
	serverDenied.authorize = func(_ context.Context, access Access) error {
		if access.Action == "completeRun" && access.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := serverDenied.Execute(ctx, deniedPrincipal, Command{OpID: "server-scope-denied", Action: "completeRun", SessionID: deniedReceipt.SessionID, RunID: "remaining-run-id", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("server completion scope = %v", err)
	}
	serverBroken, brokenPrincipal, brokenReceipt := newRun(t, "check", true)
	if err := serverBroken.repo.Transact(ctx, func(tx Transaction) error {
		var session Session
		if _, e := tx.Load(ctx, Sessions, brokenReceipt.SessionID, &session); e != nil {
			return e
		}
		session.PendingRunCount = 0
		session.Version++
		return tx.Save(ctx, Sessions, session.ID, session, session.Version-1, brokenPrincipal.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := serverBroken.Execute(ctx, brokenPrincipal, Command{OpID: "server-counter-underflow", Action: "completeRun", SessionID: brokenReceipt.SessionID, RunID: "remaining-run-id", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("server pending counter = %v", err)
	}

	planSvc, planPrincipal, planReceipt := newRun(t, "chat", false)
	planBase := Command{OpID: "bad-plan-run-id", Action: "recordRunExecution", SessionID: planReceipt.SessionID, ExpectedVersion: 2, ExpectedRunVersion: 1, RunID: "remaining-run-id", ExecutionDocument: testQueryDefinition("select 1")}
	planBase.RunID = "bad id"
	if _, err := planSvc.Execute(ctx, planPrincipal, planBase); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe plan run id = %v", err)
	}
	planBase.OpID, planBase.RunID, planBase.ResolvedParameters = "bad-plan-params", "remaining-run-id", map[string]apicontract.TypedValue{"bad name": apicontract.NewStringValue("v")}
	if _, err := planSvc.Execute(ctx, planPrincipal, planBase); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid plan parameters = %v", err)
	}
	planBase.OpID, planBase.ResolvedParameters, planBase.ExecutionDocument = "bad-plan-document", nil, json.RawMessage("{")
	if _, err := planSvc.Execute(ctx, planPrincipal, planBase); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid execution document = %v", err)
	}
	planBase.OpID, planBase.ExecutionDocument = "plan-input-auth", testQueryDefinition("select 1")
	planSvc.authorize = func(_ context.Context, access Access) error {
		if access.Action == "recordRunExecution" && access.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := planSvc.Execute(ctx, planPrincipal, planBase); !errors.Is(err, ErrForbidden) {
		t.Fatalf("plan input scope = %v", err)
	}
	planSvc2, planPrincipal2, planReceipt2 := newRun(t, "chat", false)
	planBadSchema := Command{OpID: "bad-plan-schema", Action: "recordRunExecution", SessionID: planReceipt2.SessionID, ExpectedVersion: 2, ExpectedRunVersion: 1, RunID: "remaining-run-id", ExecutionDocument: testDefinition("chat")}
	if _, err := planSvc2.Execute(ctx, planPrincipal2, planBadSchema); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong execution document kind = %v", err)
	}
	serverPlanSvc, serverPlanPrincipal, serverPlanReceipt := newRun(t, "chat", true)
	serverPlan := Command{OpID: "server-plan", Action: "recordRunExecution", SessionID: serverPlanReceipt.SessionID, ExpectedVersion: 2, ExpectedRunVersion: 1, RunID: "remaining-run-id", ExecutionDocument: testQueryDefinition("select 1")}
	if _, err := serverPlanSvc.Execute(ctx, serverPlanPrincipal, serverPlan); err != nil {
		t.Fatalf("server execution plan = %v", err)
	}
	planPrincipal2.Executor = true
	if _, err := planSvc2.Execute(ctx, planPrincipal2, Command{OpID: "wrong-authority-plan", Action: "recordRunExecution", SessionID: planReceipt2.SessionID, ExpectedVersion: 2, ExpectedRunVersion: 1, RunID: "remaining-run-id", ExecutionDocument: testQueryDefinition("select 1")}); !errors.Is(err, ErrConflict) {
		t.Fatalf("execution authority conflict = %v", err)
	}

	dashSvc, dashPrincipal, dashReceipt := newRun(t, "dashboard", false)
	dashSvc.authorize = func(_ context.Context, access Access) error {
		if access.Action == "reportDashboard" && access.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := dashSvc.Execute(ctx, dashPrincipal, Command{OpID: "dashboard-scope-denied", Action: "reportDashboard", SessionID: dashReceipt.SessionID, RunID: "remaining-run-id", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("dashboard scope = %v", err)
	}
	clientSvc, clientPrincipal, clientReceipt := newRun(t, "query", false)
	clientSvc.authorize = func(_ context.Context, access Access) error {
		if access.Action == "reportClientRun" && access.Scope != nil {
			return ErrForbidden
		}
		return nil
	}
	if _, err := clientSvc.Execute(ctx, clientPrincipal, Command{OpID: "client-scope-denied", Action: "reportClientRun", SessionID: clientReceipt.SessionID, RunID: "remaining-run-id", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client report scope = %v", err)
	}
	dashboardBroken, dashboardPrincipal, dashboardReceipt := newRun(t, "dashboard", false)
	if err := dashboardBroken.repo.Transact(ctx, func(tx Transaction) error {
		var session Session
		if _, e := tx.Load(ctx, Sessions, dashboardReceipt.SessionID, &session); e != nil {
			return e
		}
		session.PendingRunCount = 0
		session.Version++
		return tx.Save(ctx, Sessions, session.ID, session, session.Version-1, dashboardPrincipal.Actor, session.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := dashboardBroken.Execute(ctx, dashboardPrincipal, Command{OpID: "dashboard-counter-underflow", Action: "reportDashboard", SessionID: dashboardReceipt.SessionID, RunID: "remaining-run-id", ExpectedRunVersion: 1, RunState: "completed"}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("dashboard pending counter = %v", err)
	}

	// Snapshot admission must propagate a read failure before committing a bag.
	snapshotSvc, snapshotPrincipal := newTestService(t, nil)
	snapshotReceipt := createTestSession(t, snapshotSvc, snapshotPrincipal, "snapshot-read-failure", "snapshot-read-bag")
	snapshotFault := &interceptedRepository{Repository: snapshotSvc.repo, failLoad: func(collection, _ string) error {
		if collection == ContextSets {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := interceptService(t, snapshotSvc, snapshotPrincipal, snapshotFault).Execute(ctx, snapshotPrincipal, Command{OpID: "snapshot-read-fail-op", Action: "createBag", SessionID: snapshotReceipt.SessionID, ExpectedVersion: snapshotReceipt.SessionVersion, BagID: "new-snapshot-bag", Scope: &scope}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("snapshot read failure = %v", err)
	}
	if _, err := snapshotSvc.Execute(ctx, snapshotPrincipal, Command{OpID: "late-bag-failure", Action: "transitionSession", SessionID: snapshotReceipt.SessionID, ExpectedVersion: snapshotReceipt.SessionVersion, Transition: "stop", BagID: "missing-bag"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("late bag lookup = %v", err)
	}
}

func TestExecutePropagatesActionSpecificReadFailures(t *testing.T) {
	ctx := context.Background()
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	tests := []struct {
		name    string
		prepare func(*testing.T, *Service, Principal) (Principal, Command, string, string)
	}{
		{"create session lookup", func(_ *testing.T, _ *Service, p Principal) (Principal, Command, string, string) {
			return p, Command{OpID: "create-read-fail", Action: "createSession", SessionID: "create-read-fail", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "create-read-bag", Scope: &scope}, Sessions, "create-read-fail"
		}},
		{"duplicate import session lookup", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			importCmd := Command{OpID: "legacy-seed", Action: "importLegacy", LegacyImport: &LegacyImport{SourceID: "duplicate-read", Scope: scope}}
			executeOK(t, s, p, importCmd)
			importCmd.OpID = "legacy-retry-new-op"
			manifest, _, _, err := prepareLegacyImport(p, importCmd.LegacyImport)
			if err != nil {
				t.Fatal(err)
			}
			return p, importCmd, Sessions, manifest.SessionID
		}},
		{"entry read", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "entry-read-fail", "entry-read-bag")
			cmd := validPutCommand(created, created.SessionID, "entry-read-bag")
			cmd.EntryID, cmd.RevisionID = "unread-entry", "unread-revision"
			return p, cmd, Entries, "unread-entry"
		}},
		{"alias scan", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "alias-read-fail", "alias-read-bag")
			first := validPutCommand(created, created.SessionID, "alias-read-bag")
			first.EntryID, first.RevisionID, first.Alias = "alias-existing", "alias-existing-rev", "existing"
			receipt := executeOK(t, s, p, first)
			second := validPutCommand(receipt, created.SessionID, "alias-read-bag")
			second.OpID, second.ExpectedBagVersion, second.EntryID, second.RevisionID, second.Alias = "alias-scan", 2, "alias-new", "alias-new-rev", "new"
			return p, second, Entries, "alias-existing"
		}},
		{"apply context set", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "apply-read-fail", "apply-read-bag")
			return p, Command{OpID: "apply-read", Action: "applyContext", SessionID: created.SessionID, ExpectedVersion: created.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-apply-read-fail"), BagID: "apply-read-bag", ExpectedBagVersion: 1, Enabled: boolPtr(true), Scope: &scope}, ContextSets, "cs_" + opKey(p, "create-apply-read-fail")
		}},
		{"pin context set", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "pin-read-fail", "pin-read-bag")
			return p, validPinCommand(created, created.SessionID, "pin-read-run", "pin-read-input", scope, "query"), ContextSets, "cs_" + opKey(p, "create-pin-read-fail")
		}},
		{"complete run lookup", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "complete-read-fail", "complete-read-bag")
			p.Executor = true
			executeOK(t, s, p, validPinCommand(created, created.SessionID, "complete-read-run", "complete-read-input", scope, "check"))
			return p, Command{OpID: "complete-read", Action: "completeRun", SessionID: created.SessionID, RunID: "complete-read-run", ExpectedRunVersion: 1, RunState: "completed"}, Runs, "complete-read-run"
		}},
		{"run execution lookup", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "plan-read-fail", "plan-read-bag")
			pinned := executeOK(t, s, p, validPinCommand(created, created.SessionID, "plan-read-run", "plan-read-input", scope, "chat"))
			return p, Command{OpID: "plan-read", Action: "recordRunExecution", SessionID: created.SessionID, ExpectedVersion: pinned.SessionVersion, RunID: "plan-read-run", ExpectedRunVersion: 1, ExecutionDocument: testQueryDefinition("select 1")}, Runs, "plan-read-run"
		}},
		{"dashboard report lookup", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "dashboard-read-fail", "dashboard-read-bag")
			executeOK(t, s, p, validPinCommand(created, created.SessionID, "dashboard-read-run", "dashboard-read-input", scope, "dashboard"))
			return p, Command{OpID: "dashboard-read", Action: "reportDashboard", SessionID: created.SessionID, RunID: "dashboard-read-run", ExpectedRunVersion: 1, RunState: "completed"}, Runs, "dashboard-read-run"
		}},
		{"client report lookup", func(t *testing.T, s *Service, p Principal) (Principal, Command, string, string) {
			created := createTestSession(t, s, p, "client-read-fail", "client-read-bag")
			executeOK(t, s, p, validPinCommand(created, created.SessionID, "client-read-run", "client-read-input", scope, "query"))
			return p, Command{OpID: "client-read", Action: "reportClientRun", SessionID: created.SessionID, RunID: "client-read-run", Kind: "query", ExpectedRunVersion: 1, RunState: "completed"}, Runs, "client-read-run"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			base, p := newTestService(t, nil)
			principal, command, collection, id := test.prepare(t, base, p)
			fault := &interceptedRepository{Repository: base.repo, failLoad: func(c, key string) error {
				if c == collection && key == id {
					return ErrUnavailable
				}
				return nil
			}}
			if _, err := interceptService(t, base, principal, fault).Execute(ctx, principal, command); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("action-specific read error = %v", err)
			}
		})
	}
}

func mustLegacyIdentity(t *testing.T, p Principal, payload *LegacyImport) string {
	t.Helper()
	manifest, _, _, err := prepareLegacyImport(p, payload)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimPrefix(manifest.ID, "li_")
}

func TestListSessionsRejectsCrossOwnerRowsAndScopedRefValidation(t *testing.T) {
	base, p := newTestService(t, nil)
	ownerRow := json.RawMessage(`{"id":"foreign-session","owner":"bob","databaseId":"db1","projectId":"project1"}`)
	fault := &interceptedRepository{Repository: base.repo, listOverride: func(string, string, string, int) ([]json.RawMessage, string, error) {
		return []json.RawMessage{ownerRow}, "", nil
	}}
	if _, err := interceptService(t, base, p, fault).ListSessions(context.Background(), p, "", 10); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign owner row = %v", err)
	}
	created := createTestSession(t, base, p, "scoped-refs", "scope-bag")
	scope := Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}
	put := Command{OpID: "scoped-entry", Action: "putEntry", SessionID: "scoped-refs", ExpectedVersion: created.SessionVersion, BagID: "scope-bag", ExpectedBagVersion: 1, EntryID: "scope-entry", RevisionID: "scope-revision", Subject: &Subject{ConceptID: "C", FieldID: "C.ID", FieldMappingRef: "m"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("v"))}}
	executeOK(t, base, p, put)
	var v Session
	var bag Bag
	if err := base.repo.View(context.Background(), func(r Reader) error {
		_, e := r.Load(context.Background(), Sessions, "scoped-refs", &v)
		if e != nil {
			return e
		}
		_, e = r.Load(context.Background(), Bags, "scope-bag", &bag)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var first []EntryRef
	if err := base.repo.View(context.Background(), func(r Reader) error {
		var e error
		first, e = base.validateScopedRefs(context.Background(), r, p, v, scope, []EntryRef{{EntryID: "scope-entry", RevisionID: "scope-revision"}, {EntryID: "scope-entry", RevisionID: "scope-revision"}})
		if e != nil {
			return e
		}
		_, e = base.validateScopedRefs(context.Background(), r, p, v, Scope{EnvironmentID: "uat", IdentityDomain: "other"}, []EntryRef{{EntryID: "scope-entry", RevisionID: "scope-revision"}})
		if !errors.Is(e, ErrForbidden) {
			return fmt.Errorf("wrong scope: %v", e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 {
		t.Fatalf("identical refs not deduplicated: %+v", first)
	}
	if err := base.repo.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Revisions, "scope-old-revision", EntryRevision{ID: "scope-old-revision", EntryID: "scope-entry", BagID: "scope-bag"}, 0, p.Actor, "scoped-refs")
	}); err != nil {
		t.Fatal(err)
	}
	if err := base.repo.View(context.Background(), func(r Reader) error {
		var e error
		_, e = base.validateScopedRefs(context.Background(), r, p, v, scope, []EntryRef{{EntryID: "scope-entry", RevisionID: "scope-revision"}, {EntryID: "scope-entry", RevisionID: "scope-old-revision"}})
		return e
	}); err == nil || !errors.Is(err, ErrInvalid) {
		t.Fatalf("conflicting revisions for one scoped entry = %v", err)
	}
}

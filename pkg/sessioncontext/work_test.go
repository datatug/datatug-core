package sessioncontext

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

func workTestService(t *testing.T, repo Repository, id, owner string, policy Authorizer) *Service {
	t.Helper()
	if policy == nil {
		policy = func(context.Context, Access) error { return nil }
	}
	s, err := NewWorkService(repo, WorkBinding{WorkItemID: "work-" + id, SessionID: id, IndexOwner: owner, DatabaseID: "db1", ProjectID: "project1"}, policy)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestWorkSessionsPreservePrivateBoundaryAndRealActor(t *testing.T) {
	ctx := context.Background()
	private, alice := newTestService(t, nil)
	work := workTestService(t, private.repo, "shared", alice.Actor, nil)
	create := Command{OpID: "create-shared", Action: "createSession", SessionID: "shared", BagID: "shared-bag", DatabaseID: alice.DatabaseID, ProjectID: alice.ProjectID, Title: "Incident", Scope: &Scope{"uat", "tenant-a"}}
	created := executeOK(t, work, alice, create)
	snap, err := work.GetSnapshot(ctx, alice, "shared")
	if err != nil || snap.Session.WorkItemID != "work-shared" {
		t.Fatalf("snapshot: %+v %v", snap, err)
	}
	if _, err = private.GetSnapshot(ctx, alice, "shared"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private read: %v", err)
	}
	if _, err = private.Execute(ctx, alice, create); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private replay: %v", err)
	}
	list, err := private.ListSessions(ctx, alice, "", 20)
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("private list leaked: %+v %v", list, err)
	}
	bob := alice
	bob.Actor = "bob"
	put := Command{OpID: "bob-put", Action: "putEntry", SessionID: "shared", ExpectedVersion: created.SessionVersion, BagID: "shared-bag", ExpectedBagVersion: 1, EntryID: "order", RevisionID: "order-v1", Subject: &Subject{ConceptID: "Order", FieldID: "id", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("11045"))}}
	r := executeOK(t, work, bob, put)
	if r.Actor != "bob" {
		t.Fatalf("actor impersonated: %+v", r)
	}
	snap, err = work.GetSnapshot(ctx, bob, "shared")
	if err != nil || snap.Session.Owner != "alice" {
		t.Fatalf("owner drift: %+v %v", snap, err)
	}
	revisions, err := work.ListEntryRevisions(ctx, bob, "shared", "order", "", 20)
	if err != nil || len(revisions.Items) != 1 {
		t.Fatalf("collaborator revisions: %+v %v", revisions, err)
	}
	err = private.repo.View(ctx, func(reader Reader) error {
		for _, collection := range []string{Sessions, Bags, Entries, Revisions} {
			rows, _, e := reader.List(ctx, collection, "bob", "", 20)
			if e != nil {
				return e
			}
			if len(rows) != 0 {
				t.Fatalf("index owner drift in %s", collection)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"pinInput", "transitionSession", "importLegacy", "completeRun", "reportClientRun"} {
		if _, err = work.Execute(ctx, bob, Command{Action: action, OpID: "denied-" + action, SessionID: "shared"}); err == nil {
			t.Fatalf("shared action %s accepted", action)
		}
	}
	if _, err = work.ListSessions(ctx, bob, "", 20); !errors.Is(err, ErrForbidden) {
		t.Fatalf("shared private listing: %v", err)
	}
	denied := workTestService(t, private.repo, "shared", alice.Actor, func(context.Context, Access) error { return ErrForbidden })
	if _, err = denied.GetSnapshot(ctx, bob, "shared"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked: %v", err)
	}
	if _, err = denied.Execute(ctx, bob, put); !errors.Is(err, ErrForbidden) {
		t.Fatalf("revoked replay: %v", err)
	}
	wrong := workTestService(t, private.repo, "shared", "bob", nil)
	if _, err = wrong.GetSnapshot(ctx, bob, "shared"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("forged index owner: %v", err)
	}
	// Ordinary private sessions never become Work sessions through the policy.
	createTestSession(t, private, alice, "private", "private-bag")
	bound := workTestService(t, private.repo, "private", alice.Actor, nil)
	if _, err = bound.GetSnapshot(ctx, bob, "private"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private promoted implicitly: %v", err)
	}
}

func TestCopyWorkContextIsolationRefreshConflictAndPrivateExecution(t *testing.T) {
	ctx := context.Background()
	private, alice := newTestService(t, nil)
	parent := workTestService(t, private.repo, "parent", "alice", nil)
	child := workTestService(t, private.repo, "child", "alice", nil)
	createTestSession(t, parent, alice, "parent", "parent-bag")
	createTestSession(t, child, alice, "child", "child-bag")
	put := Command{OpID: "parent-value", Action: "putEntry", SessionID: "parent", ExpectedVersion: 1, BagID: "parent-bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Alias: "Order", Subject: &Subject{ConceptID: "Order", FieldID: "id", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("11045"))}}
	executeOK(t, parent, alice, put)
	bob := alice
	bob.Actor = "bob"
	cmd := CopyContextCommand{OpID: "copy", SourceSessionID: "parent", DestinationSessionID: "child", ExpectedSourceVersion: 2, ExpectedDestinationVersion: 1, Selection: []EntryRef{{"entry", "revision"}}}
	copied, err := CopyContext(ctx, bob, parent, child, cmd)
	if err != nil {
		t.Fatal(err)
	}
	if copied.Receipt.Actor != "bob" {
		t.Fatal("copied with wrong actor")
	}
	replay, err := CopyContext(ctx, bob, parent, child, cmd)
	if err != nil || replay.Receipt != copied.Receipt {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	snapshot, err := child.GetSnapshot(ctx, bob, "child")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Entries) != 1 || snapshot.Revisions[0].Source == nil || snapshot.Revisions[0].Source.RevisionID != "revision" || len(snapshot.ContextSet.Slots) != 0 {
		t.Fatalf("copy/provenance/apply: %+v", snapshot)
	}
	ce := snapshot.Entries[0]
	var cb Bag
	for _, b := range snapshot.Bags {
		if b.ID == ce.BagID {
			cb = b
		}
	}
	edit := put
	edit.OpID = "child-local"
	edit.SessionID = "child"
	edit.ExpectedVersion = snapshot.Session.Version
	edit.BagID = cb.ID
	edit.ExpectedBagVersion = cb.Version
	edit.EntryID = ce.ID
	edit.ExpectedEntryVersion = ce.Version
	edit.RevisionID = "child-local-revision"
	edit.Value = &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("11062"))}
	executeOK(t, child, bob, edit)
	original, err := parent.GetSnapshot(ctx, alice, "parent")
	if err != nil || original.Revisions[0].Value.Scalar.Str != "11045" {
		t.Fatalf("child changed parent: %+v %v", original, err)
	}
	cmd.OpID = "refresh"
	cmd.ExpectedDestinationVersion = 3
	if _, err = CopyContext(ctx, bob, parent, child, cmd); !errors.Is(err, ErrConflict) {
		t.Fatalf("local override overwritten: %v", err)
	}
	after, err := child.GetSnapshot(ctx, bob, "child")
	if err != nil || after.Session.Version != 3 || after.Revisions[0].Value.Scalar.Str != "11062" {
		t.Fatalf("failed copy changed child: %+v %v", after, err)
	}
	createTestSession(t, private, bob, "execution", "execution-bag")
	cmd.OpID = "private-execution"
	cmd.DestinationSessionID = "execution"
	cmd.ExpectedDestinationVersion = 1
	if _, err = CopyContext(ctx, bob, parent, private, cmd); err != nil {
		t.Fatal(err)
	}
	execution, err := private.GetSnapshot(ctx, bob, "execution")
	if err != nil || execution.Session.WorkItemID != "" || execution.Session.Owner != "bob" || len(execution.Entries) != 1 {
		t.Fatalf("private snapshot: %+v %v", execution, err)
	}
	if _, err = private.GetSnapshot(ctx, alice, "execution"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("private execution leaked: %v", err)
	}
	cmd.OpID = "stale-source"
	cmd.ExpectedSourceVersion = 1
	cmd.ExpectedDestinationVersion = 2
	if _, err = CopyContext(ctx, bob, parent, private, cmd); !errors.Is(err, ErrConflict) {
		t.Fatalf("source CAS: %v", err)
	}
	cmd.OpID = "stale-destination"
	cmd.ExpectedSourceVersion = 2
	cmd.ExpectedDestinationVersion = 1
	if _, err = CopyContext(ctx, bob, parent, private, cmd); !errors.Is(err, ErrConflict) {
		t.Fatalf("destination CAS: %v", err)
	}
}

func TestBufferedContextTransactionAtomicityAndExplicitFlush(t *testing.T) {
	ctx := context.Background()
	private, p := newTestService(t, nil)
	raw := private.repo.(*DALRepository)
	sentinel := errors.New("abort enclosing Work transaction")
	err := raw.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		buffered, e := raw.BufferInTransaction(tx)
		if e != nil {
			return e
		}
		s := workTestService(t, buffered, "atomic", p.Actor, nil)
		createTestSession(t, s, p, "atomic", "atomic-bag")
		// The DALGO transaction can still perform reads before explicit flush.
		if _, e = s.GetSnapshot(ctx, p, "atomic"); e != nil {
			return e
		}
		if e = buffered.Flush(ctx); e != nil {
			return e
		}
		if e = buffered.View(ctx, func(Reader) error { return nil }); !errors.Is(e, ErrUnavailable) {
			t.Fatalf("read after flush: %v", e)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	s := workTestService(t, raw, "atomic", p.Actor, nil)
	if _, err = s.GetSnapshot(ctx, p, "atomic"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("aborted write persisted: %v", err)
	}
	err = raw.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		b, e := raw.BufferInTransaction(tx)
		if e != nil {
			return e
		}
		s := workTestService(t, b, "atomic", p.Actor, nil)
		createTestSession(t, s, p, "atomic", "atomic-bag")
		return b.Flush(ctx)
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.GetSnapshot(ctx, p, "atomic"); err != nil {
		t.Fatal(err)
	}
	err = raw.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		b, e := raw.BufferInTransaction(tx)
		if e != nil {
			return e
		}
		if e = b.Transact(ctx, func(Transaction) error { return sentinel }); !errors.Is(e, sentinel) {
			t.Fatal(e)
		}
		return b.Flush(ctx)
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("failed command flush accepted: %v", err)
	}
}

func TestWorkCopyRefreshReauthorizesScopesAndRetainsAppliedRevision(t *testing.T) {
	ctx := context.Background()
	private, p := newTestService(t, nil)
	source := workTestService(t, private.repo, "source", p.Actor, nil)
	destination := workTestService(t, private.repo, "destination", p.Actor, nil)
	createTestSession(t, source, p, "source", "source-bag")
	createTestSession(t, destination, p, "destination", "destination-bag")
	put := Command{OpID: "initial", Action: "putEntry", SessionID: "source", ExpectedVersion: 1, BagID: "source-bag", ExpectedBagVersion: 1, EntryID: "e", RevisionID: "r1", Alias: "Order", Subject: &Subject{ConceptID: "Order", FieldID: "id", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}}
	executeOK(t, source, p, put)
	copy := CopyContextCommand{OpID: "initial-copy", SourceSessionID: "source", DestinationSessionID: "destination", ExpectedSourceVersion: 2, ExpectedDestinationVersion: 1, Selection: []EntryRef{{"e", "r1"}}}
	if _, err := CopyContext(ctx, p, source, destination, copy); err != nil {
		t.Fatal(err)
	}
	before, err := destination.GetSnapshot(ctx, p, "destination")
	if err != nil {
		t.Fatal(err)
	}
	entry := before.Entries[0]
	bagID := entry.BagID
	applied := executeOK(t, destination, p, Command{OpID: "apply-copied", Action: "applyContext", SessionID: "destination", ExpectedVersion: 2, BagID: bagID, ExpectedBagVersion: 1, ExpectedContextSetID: before.ContextSet.ID, Enabled: boolPtr(true), Scope: &Scope{"uat", "tenant-a"}, Selection: []EntryRef{{entry.ID, entry.CurrentRevisionID}}})
	put.OpID = "update-source"
	put.ExpectedVersion = 2
	put.ExpectedBagVersion = 2
	put.ExpectedEntryVersion = 1
	put.RevisionID = "r2"
	put.Value = &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("2"))}
	executeOK(t, source, p, put)
	copy.OpID = "refresh"
	copy.ExpectedSourceVersion = 3
	copy.ExpectedDestinationVersion = applied.SessionVersion
	copy.Selection = []EntryRef{{"e", "r2"}}
	if _, err = CopyContext(ctx, p, source, destination, copy); err != nil {
		t.Fatal(err)
	}
	after, err := destination.GetSnapshot(ctx, p, "destination")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Entries) != 1 || len(after.Revisions) != 2 || len(after.ContextSet.Slots) != 1 || after.ContextSet.Slots[0].Entries[0].RevisionID != entry.CurrentRevisionID {
		t.Fatalf("refresh lost applied revision: %+v", after)
	}
	if after.Entries[0].CurrentRevisionID == entry.CurrentRevisionID {
		t.Fatal("refresh did not advance head")
	}
	denied := workTestService(t, private.repo, "source", p.Actor, func(_ context.Context, a Access) error {
		if a.Scope != nil {
			return ErrForbidden
		}
		return nil
	})
	if _, err = CopyContext(ctx, p, denied, destination, copy); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay bypassed scope revocation: %v", err)
	}
	foreign := p
	foreign.ProjectID = "other"
	if _, err = CopyContext(ctx, foreign, source, destination, copy); !errors.Is(err, ErrForbidden) {
		t.Fatalf("cross-project copy: %v", err)
	}
	different, _ := newTestService(t, nil)
	if _, err = CopyContext(ctx, p, source, different, copy); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-transaction copy: %v", err)
	}
	copy.OpID = "duplicate-selection"
	copy.Selection = append(copy.Selection, copy.Selection[0])
	if _, err = CopyContext(ctx, p, source, destination, copy); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate reference: %v", err)
	}
}

func TestWorkServiceRejectsInvalidBindingAndNativePin(t *testing.T) {
	private, p := newTestService(t, nil)
	if _, err := NewWorkService(private.repo, WorkBinding{}, func(context.Context, Access) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	work := workTestService(t, private.repo, "shared", p.Actor, nil)
	if _, err := work.PinDetectiveInput(context.Background(), p, PinDetectiveInputCommand{}); err == nil {
		t.Fatal("native execution accepted")
	}
	if _, _, err := work.ReadInput(context.Background(), p, "missing"); err == nil {
		t.Fatal("shared private input accepted")
	}
	if _, err := work.Execute(context.Background(), p, Command{Action: "createSession", OpID: "outside", SessionID: "foreign"}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("binding escaped: %v", err)
	}
	if _, err := CopyContext(context.Background(), p, nil, work, CopyContextCommand{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	raw := private.repo.(*DALRepository)
	if _, err := raw.BufferInTransaction(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var nilRepo *DALRepository
	if _, err := nilRepo.BufferInTransaction(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

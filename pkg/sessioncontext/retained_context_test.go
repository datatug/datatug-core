package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func retainedContextFixture(t *testing.T) (RetainedInventory, func()) {
	t.Helper()
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "session", "bag")
	scope := Scope{"uat", "tenant-a"}
	subject := Subject{ConceptID: "external-concept", FieldID: "field-a", FieldMappingRef: "external-mapping", GraphResource: "meaninggraph://registry/graph", GraphRevision: "graph-revision", ModelRef: "modelspec://registry/model", ModelRevision: "model-revision", Fields: []string{"field-a"}}
	put := Command{Action: "putEntry", OpID: "put-a", SessionID: "session", ExpectedVersion: created.SessionVersion, BagID: "bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision-a", Subject: &subject, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}}
	a := executeOK(t, svc, p, put)
	snapshot, err := svc.GetSnapshot(context.Background(), p, "session")
	if err != nil {
		t.Fatal(err)
	}
	apply := Command{Action: "applyContext", OpID: "apply-a", SessionID: "session", ExpectedVersion: a.SessionVersion, BagID: "bag", ExpectedBagVersion: 2, ExpectedContextSetID: snapshot.ContextSet.ID, Scope: &scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: "entry", RevisionID: "revision-a"}}}
	applied := executeOK(t, svc, p, apply)
	subject.FieldID = "field-b"
	subject.Fields = []string{"field-b"}
	put.OpID, put.RevisionID, put.ExpectedVersion, put.ExpectedBagVersion, put.ExpectedEntryVersion = "put-b", "revision-b", applied.SessionVersion, 2, 1
	edited := executeOK(t, svc, p, put)
	bob := p
	bob.Actor = "bob"
	createTestSession(t, svc, bob, "bob-session", "bob-bag")
	read := func() RetainedInventory {
		inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}
	before := read()
	clear := func() {
		snapshot, err := svc.GetSnapshot(context.Background(), p, "session")
		if err != nil {
			t.Fatal(err)
		}
		apply.OpID, apply.ExpectedVersion, apply.ExpectedBagVersion, apply.ExpectedSlotVersion, apply.ExpectedContextSetID = "apply-b", edited.SessionVersion, 3, 1, snapshot.ContextSet.ID
		apply.Selection = []EntryRef{{EntryID: "entry", RevisionID: "revision-b"}}
		reapplied := executeOK(t, svc, p, apply)
		executeOK(t, svc, p, Command{Action: "clearBag", OpID: "clear", SessionID: "session", ExpectedVersion: reapplied.SessionVersion, BagID: "bag", ExpectedBagVersion: 3, ExpectedSlotVersion: 2, Scope: &scope})
		after, err := ProjectRetainedContextBindings(context.Background(), read())
		if err != nil {
			t.Fatal(err)
		}
		retained := map[string]bool{}
		for _, binding := range after {
			if binding.Collection == Entries && binding.ID == "entry" && binding.Active {
				t.Fatal("cleared head remained active")
			}
			if binding.Collection == Revisions {
				if binding.Active {
					t.Fatal("immutable revision became active")
				}
				retained[binding.ID] = true
			}
			if binding.Collection == ContextSets && binding.Active && len(binding.Subjects) != 0 {
				t.Fatal("cleared current context retained an active selection")
			}
		}
		if !retained["revision-a"] || !retained["revision-b"] {
			t.Fatal("clear lost retained revisions")
		}
	}
	return before, clear
}

func TestRetainedContextBindingsPreserveAppliedHistoryAndAllOwners(t *testing.T) {
	inventory, clear := retainedContextFixture(t)
	bindings, err := ProjectRetainedContextBindings(context.Background(), inventory)
	if err != nil {
		t.Fatal(err)
	}
	owners, activeFields, historicalFields := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, binding := range bindings {
		owners[binding.Owner] = true
		if binding.Namespace != inventory.Namespace {
			t.Fatal("storage namespace changed")
		}
		for _, subject := range binding.Subjects {
			if subject.Scope != (Scope{"uat", "tenant-a"}) || subject.Subject.GraphResource != "meaninggraph://registry/graph" || subject.Subject.ModelRef != "modelspec://registry/model" {
				t.Fatal("semantic descriptor lost scope or external identity")
			}
			if binding.Active {
				activeFields[subject.Subject.FieldID] = true
			} else {
				historicalFields[subject.Subject.FieldID] = true
			}
		}
	}
	if !owners["alice"] || !owners["bob"] || !activeFields["field-a"] || !activeFields["field-b"] || !historicalFields["field-a"] {
		t.Fatalf("current/applied/history/owners lost: %v %v %v", owners, activeFields, historicalFields)
	}
	for i := range bindings {
		if len(bindings[i].Subjects) > 0 {
			bindings[i].Subjects[0].Subject.Fields[0] = "changed"
		}
	}
	again, err := ProjectRetainedContextBindings(context.Background(), inventory)
	if err != nil {
		t.Fatal(err)
	}
	for _, binding := range again {
		for _, subject := range binding.Subjects {
			if subject.Subject.Fields[0] == "changed" {
				t.Fatal("output mutated retained input")
			}
		}
	}
	clear()
}

func mutateRetainedContext(t *testing.T, inventory RetainedInventory, collection, id string, mutate func(*RetainedDocument)) RetainedInventory {
	t.Helper()
	raw, err := json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	var copy RetainedInventory
	if err = json.Unmarshal(raw, &copy); err != nil {
		t.Fatal(err)
	}
	for i := range copy.Documents {
		if d := &copy.Documents[i]; d.Collection == collection && d.ID == id {
			mutate(d)
			return copy
		}
	}
	t.Fatal("fixture document missing")
	return RetainedInventory{}
}

func TestRetainedContextBindingsRejectUnknownAndBrokenJoinsWithoutPartialOutput(t *testing.T) {
	inventory, _ := retainedContextFixture(t)
	for _, scenario := range []string{"owner", "version", "parent", "cycle", "source", "schema", "scope"} {
		t.Run(scenario, func(t *testing.T) {
			collection, id := Revisions, "revision-a"
			if scenario == "scope" {
				collection, id = Bags, "bag"
			}
			bad := mutateRetainedContext(t, inventory, collection, id, func(d *RetainedDocument) {
				if scenario == "owner" {
					d.Owner = "bob"
					return
				}
				if scenario == "version" {
					d.Version++
					return
				}
				var body map[string]any
				if err := json.Unmarshal(d.Payload, &body); err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "parent":
					body["parentRevisionId"] = "missing"
				case "cycle":
					body["parentRevisionId"] = "revision-b"
				case "source":
					body["provenance"] = "work-context-copy"
					body["source"] = map[string]string{"sessionId": "bob-session", "bagId": "bob-bag", "entryId": "entry", "revisionId": "revision-b"}
				case "schema":
					body["futureReference"] = "unknown"
				case "scope":
					body["scope"] = map[string]string{"environmentId": "foreign", "identityDomain": "tenant-a"}
				}
				d.Payload, _ = json.Marshal(body)
			})
			if out, err := ProjectRetainedContextBindings(context.Background(), bad); !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatalf("partial or accepted invalid context: len=%d error=%v", len(out), err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, err := ProjectRetainedContextBindings(ctx, inventory); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal("canceled inventory returned bindings")
	}
	if out, err := ProjectRetainedContextBindings(nil, inventory); !errors.Is(err, ErrInvalid) || out != nil {
		t.Fatal("nil context accepted")
	}
	first, _ := ProjectRetainedContextBindings(context.Background(), inventory)
	second, _ := ProjectRetainedContextBindings(context.Background(), inventory)
	if !reflect.DeepEqual(first, second) {
		t.Fatal("source binding order is not deterministic")
	}
}

func TestRetainedContextBindingsBoundFlattenedDescriptorAmplification(t *testing.T) {
	inventory, _ := retainedContextFixture(t)
	inventory = mutateRetainedContext(t, inventory, Revisions, "revision-a", func(d *RetainedDocument) {
		var value EntryRevision
		if err := json.Unmarshal(d.Payload, &value); err != nil {
			t.Fatal(err)
		}
		value.Subject.FieldMappingRef = strings.Repeat("mapping", 70000)
		d.Payload, _ = json.Marshal(value)
	})
	if _, err := ProjectRetainedContextBindings(context.Background(), inventory); err != nil {
		t.Fatalf("bounded descriptor fixture rejected before amplification: %v", err)
	}
	var applied AppliedContextSet
	var base RetainedDocument
	for _, d := range inventory.Documents {
		if d.Collection != ContextSets {
			continue
		}
		var set AppliedContextSet
		if err := json.Unmarshal(d.Payload, &set); err != nil {
			t.Fatal(err)
		}
		if len(set.Slots) == 1 && len(set.Slots[0].Entries) == 1 {
			applied, base = set, d
			break
		}
	}
	if base.ID == "" {
		t.Fatal("applied context fixture missing")
	}
	for i := range 40 {
		d := base
		d.ID = fmt.Sprintf("historical-context-%d", i)
		set := applied
		set.ID = d.ID
		d.Payload, _ = json.Marshal(set)
		inventory.Documents = append(inventory.Documents, d)
	}
	if out, err := ProjectRetainedContextBindings(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatalf("unbounded flattened descriptors: %d %v", len(out), err)
	}
}

func TestRetainedContextBindingsJoinWorkCopiesAcrossOwnersAndSourceEdits(t *testing.T) {
	ctx := context.Background()
	repo, db := retainedInventoryFixture(t)
	allow := func(context.Context, Access) error { return nil }
	private, err := NewService(repo, allow)
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	work, err := NewWorkService(repo, WorkBinding{WorkItemID: "work", SessionID: "shared", IndexOwner: "alice", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID}, allow)
	if err != nil {
		t.Fatal(err)
	}
	created := createTestSession(t, work, p, "shared", "shared-bag")
	subject := Subject{ConceptID: "external", FieldID: "field-a", FieldMappingRef: "external-mapping"}
	put := Command{Action: "putEntry", OpID: "work-a", SessionID: "shared", ExpectedVersion: created.SessionVersion, BagID: "shared-bag", ExpectedBagVersion: 1, EntryID: "shared-entry", RevisionID: "work-revision-a", Subject: &subject, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("99909378"))}}
	a := executeOK(t, work, p, put)
	bob := p
	bob.Actor = "bob"
	destination := createTestSession(t, private, bob, "private", "private-bag")
	command := CopyContextCommand{OpID: "copy-a", SourceSessionID: "shared", DestinationSessionID: "private", ExpectedSourceVersion: a.SessionVersion, ExpectedDestinationVersion: destination.SessionVersion, Selection: []EntryRef{{EntryID: "shared-entry", RevisionID: "work-revision-a"}}}
	copied, err := CopyContext(ctx, bob, work, private, command)
	if err != nil {
		t.Fatal(err)
	}
	subject.FieldID = "field-b"
	put.OpID, put.RevisionID, put.ExpectedVersion, put.ExpectedBagVersion, put.ExpectedEntryVersion = "work-b", "work-revision-b", a.SessionVersion, 2, 1
	b := executeOK(t, work, p, put)
	command.OpID, command.ExpectedSourceVersion, command.ExpectedDestinationVersion = "copy-b", b.SessionVersion, copied.Receipt.SessionVersion
	command.Selection = []EntryRef{{EntryID: "shared-entry", RevisionID: "work-revision-b"}}
	if _, err = CopyContext(ctx, bob, work, private, command); err != nil {
		t.Fatal(err)
	}
	subject.FieldID = "field-c"
	put.OpID, put.RevisionID, put.ExpectedVersion, put.ExpectedBagVersion, put.ExpectedEntryVersion = "work-c", "work-revision-c", b.SessionVersion, 3, 2
	executeOK(t, work, p, put)
	inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := ProjectRetainedContextBindings(ctx, inventory)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, binding := range bindings {
		if binding.SessionID == "shared" && (binding.WorkItemID != "work" || binding.Owner != "alice") {
			t.Fatal("Work creator partition lost")
		}
		if binding.Source != nil {
			if binding.Owner != "bob" || binding.Source.SessionID != "shared" || binding.Source.EntryID != "shared-entry" {
				t.Fatal("copy provenance confused source and destination owners")
			}
			seen[binding.Source.RevisionID] = true
		}
	}
	if !seen["work-revision-a"] || !seen["work-revision-b"] {
		t.Fatal("copy refresh lost older source references")
	}
	encoded, _ := json.Marshal(bindings)
	if strings.Contains(string(encoded), "99909378") {
		t.Fatal("source values escaped private reference descriptors")
	}
	sources, err := ProjectRetainedSessionSources(ctx, inventory)
	if err != nil {
		t.Fatal(err)
	}
	copyAudits := 0
	for _, item := range sources.Provenance {
		if item.Collection == Audit && item.Action == "copyContext" {
			copyAudits++
			if item.Owner != "bob" || item.SessionID != "private" || len(item.Resources) != 2 || item.Resources[1].SessionID != "shared" {
				t.Fatal("copy audit lost distinct source and destination sessions")
			}
		}
	}
	if copyAudits != 2 {
		t.Fatal("older copy audit omitted")
	}
	wrongScope := mutateRetainedContext(t, inventory, Bags, "shared-bag", func(d *RetainedDocument) {
		var bag Bag
		if err := json.Unmarshal(d.Payload, &bag); err != nil {
			t.Fatal(err)
		}
		bag.Scope = Scope{"other-environment", "other-domain"}
		d.Payload, _ = json.Marshal(bag)
	})
	if out, err := ProjectRetainedContextBindings(ctx, wrongScope); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("copy provenance accepted a different source Scope")
	}
}

type cancelAtContextCheck struct {
	context.Context
	checks, at int
	cancel     context.CancelFunc
}

func (c *cancelAtContextCheck) Err() error {
	c.checks++
	if c.at != 0 && c.checks == c.at {
		c.cancel()
	}
	return c.Context.Err()
}

func TestRetainedContextBindingsCancellationAtCompletionReturnsNoOutput(t *testing.T) {
	inventory, _ := retainedContextFixture(t)
	probe := &cancelAtContextCheck{Context: context.Background()}
	if _, err := ProjectRetainedContextBindings(probe, inventory); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	last := &cancelAtContextCheck{Context: ctx, at: probe.checks, cancel: cancel}
	if out, err := ProjectRetainedContextBindings(last, inventory); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatalf("completion cancellation returned %d bindings and %v", len(out), err)
	}
}

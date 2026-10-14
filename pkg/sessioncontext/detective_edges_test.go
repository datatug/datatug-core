package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

func TestDetectivePinMalformedCaseAndSecondaryLoadFailures(t *testing.T) {
	for _, value := range []detective.InputValue{{Set: &apicontract.TypedValueSet{}}, {}} {
		s, p := pinService(t)
		c := pinCommand(t, s, p)
		c.Case = &detective.Case{ID: "case", Version: 1, Status: "active", Context: map[string]detective.InputValue{"bad": value}}
		c.Pin.Dispatch.CaseID = "case"
		c.Pin.Dispatch.CaseVersion = 1
		c.Pin.Dispatch.CaseContextDigest = strings.Repeat("0", 64)
		if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		assertNoPin(t, s, p, c)
	}
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	c.Case = &detective.Case{ID: "case", Version: 1, Status: "active"}
	bindPin(t, p, &c)
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatalf("nil Case context: %v", err)
	}
	for _, collection := range []string{Revisions, Bags} {
		t.Run(collection, func(t *testing.T) {
			s, p := pinService(t)
			c := bagPinCommand(t, s, p)
			base := s.repo
			loads := 0
			target := 2
			if collection == Bags {
				target = 3
			}
			s.repo = &interceptedRepository{Repository: base, failLoad: func(actual, id string) error {
				if actual == collection && (id == "r1" || id == "b1") {
					loads++
					if loads == target {
						return ErrUnavailable
					}
				}
				return nil
			}}
			if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrUnavailable) {
				t.Fatalf("late read not rejected: %v loads=%d", err, loads)
			}
			s.repo = base
			assertNoPin(t, s, p, c)
		})
	}
	if containsEntryRef([]EntryRef{{EntryID: "one", RevisionID: "r"}}, "two", "r") {
		t.Fatal("mismatched reference accepted")
	}
}

func TestDetectiveReadRejectsDeletedAndLegacyInputs(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	if err := s.repo.Transact(context.Background(), func(tx Transaction) error {
		var v Session
		version, err := tx.Load(context.Background(), Sessions, "s1", &v)
		if err != nil {
			return err
		}
		v.State = "deleted"
		v.Version++
		return tx.Save(context.Background(), Sessions, v.ID, v, version, p.Actor, v.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run"); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("deleted pin read: %v", err)
	}
	s, p = newTestService(t, nil)
	created := createTestSession(t, s, p, "s1", "b1")
	executeOK(t, s, p, Command{OpID: "legacy-pin", Action: "pinInput", SessionID: "s1", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "q", DefinitionRevision: "q1", DefinitionDocument: testQueryDefinition("select 1"), Scope: &Scope{"uat", "tenant-a"}})
	if _, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("legacy input reclassified: %v", err)
	}
}

func TestRedactionMarkerBlocksRecreationAndContentListingErrors(t *testing.T) {
	s, p := pinService(t)
	if err := s.repo.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Redactions, "s1", SessionRedaction{SessionID: "s1"}, 0, p.Actor, "s1")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Execute(context.Background(), p, Command{OpID: "create", Action: "createSession", SessionID: "s1", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, BagID: "b1", Scope: &Scope{"uat", "tenant-a"}}); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	s, p = pinService(t)
	createTestSession(t, s, p, "s1", "b1")
	s.repo = &interceptedRepository{Repository: s.repo, failLoad: func(collection, _ string) error {
		if collection == Redactions {
			return ErrUnavailable
		}
		return nil
	}}
	if _, err := s.ListSessions(context.Background(), p, "", 10); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestNativeNamespaceSeparatesCollidingPrivateSessions(t *testing.T) {
	mounted := testMountedDatabase{"db1", dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	services := []*Service{}
	for _, space := range []string{"a", "b"} {
		repo, err := NewNativePinRepository(mounted, record.NewKeyWithID("spaces", space), "host", space, "project1")
		if err != nil {
			t.Fatal(err)
		}
		s, err := NewNativePinService(repo, func(context.Context, Access) error { return nil })
		if err != nil {
			t.Fatal(err)
		}
		services = append(services, s)
	}
	p := Principal{Actor: "alice", DatabaseID: "db1", ProjectID: "project1", Executor: true}
	a := pinCommand(t, services[0], p)
	b := pinCommand(t, services[1], p)
	if _, err := services[0].PinDetectiveInput(context.Background(), p, a); err != nil {
		t.Fatal(err)
	}
	if _, err := services[1].PinDetectiveInput(context.Background(), p, a); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Space exchanged private pin: %v", err)
	}
	assertNoPin(t, services[1], p, b)
	if _, err := services[1].ReadDetectiveDispatch(context.Background(), p, "s1", "run"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	for _, key := range []*record.Key{record.NewKeyWithID("spaces", ""), record.NewKeyWithID("spaces", strings.Repeat("a", 513)), record.NewKeyWithID(strings.Repeat("a", 129), "a")} {
		if _, err := NewNativePinRepository(mounted, key, "host", "a", "project1"); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
	// Delimited strings can be identical while the real key hierarchy differs.
	x, _ := NewNativePinRepository(mounted, record.NewKeyWithID("a/b", "c"), "host", "a", "project1")
	y, _ := NewNativePinRepository(mounted, record.NewKeyWithID("a", "b/c"), "host", "a", "project1")
	if x.NativeNamespace() == y.NativeNamespace() {
		t.Fatal("ambiguous private parent serialization")
	}
}

func TestDetectiveDefinitionCanonicalGrowthBudgets(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	d := c.Pin.Definition
	d.Queries = append([]DetectiveAcceptedQueryPin(nil), d.Queries...)
	query := `{"id":"q","title":"Q","type":"SQL","text":"` + strings.Repeat("a", MaxDetectiveQueryBytes-len(`{"id":"q","title":"Q","type":"SQL","text":""}`)) + `"}`
	if len(query) > MaxDetectiveQueryBytes {
		t.Fatal("bad fixture")
	}
	d.Queries[0].DefinitionJSON = json.RawMessage(query)
	if _, _, err := CanonicalDetectiveDefinition(context.Background(), d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("canonical query growth over budget accepted: %v", err)
	}
	d = c.Pin.Definition
	d.Check.Description = "a"
	raw, _ := json.Marshal(d)
	d.Check.Description = strings.Repeat("a", MaxDetectiveDefinitionBytes-len(raw)+1)
	raw, _ = json.Marshal(d)
	if len(raw) > MaxDetectiveDefinitionBytes {
		t.Fatal("bad aggregate fixture")
	}
	if _, _, err := CanonicalDetectiveDefinition(context.Background(), d); !errors.Is(err, ErrInvalid) {
		t.Fatalf("canonical aggregate growth accepted: %v bytes=%d", err, len(raw))
	}
}

func TestDetectivePinRejectsForeignContextSetBeforeSaving(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	if err := s.repo.Transact(context.Background(), func(tx Transaction) error {
		var set AppliedContextSet
		version, err := tx.Load(context.Background(), ContextSets, c.ExpectedContextSetID, &set)
		if err != nil {
			return err
		}
		set.SessionID = "foreign"
		return tx.Save(context.Background(), ContextSets, set.ID, set, version, p.Actor, c.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("foreign context set: %v", err)
	}
	assertNoPin(t, s, p, c)
}

func TestDetectiveSetMappingCanonicalizesWithoutChangingBag(t *testing.T) {
	values := []apicontract.TypedValue{apicontract.NewIntegerValue("2"), apicontract.NewIntegerValue("1"), apicontract.NewIntegerValue("2")}
	got, err := detectiveBagValue(ValueEnvelope{SchemaVersion: 1, Kind: "set", Values: values})
	if err != nil || len(got.Set.Values) != 2 || got.Set.Values[0].Str != "1" || values[0].Str != "2" {
		t.Fatalf("set normalization: %v", err)
	}
	if _, err = detectiveBagValue(ValueEnvelope{Kind: "set", Values: []apicontract.TypedValue{{Type: "bad"}}}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestDetectivePinCanonicalAttachmentReplayAndBounds(t *testing.T) {
	s, p := pinService(t)
	c := bagPinCommand(t, s, p)
	c.Attachments = append(c.Attachments, EntryRef{EntryID: "e1", RevisionID: "r1"}, c.Attachments[0])
	bindPin(t, p, &c)
	first, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	c.Attachments = []EntryRef{{EntryID: "e1", RevisionID: "r1"}, {EntryID: "e2", RevisionID: "r2"}}
	second, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil || first.Receipt.PayloadDigest != second.Receipt.PayloadDigest {
		t.Fatalf("selection ordering changed operation: %v", err)
	}
	for _, refs := range [][]EntryRef{make([]EntryRef, apicontract.MaxSessionInputEntryRefs+1), {{EntryID: "../bad", RevisionID: "r1"}}, {{EntryID: "e1", RevisionID: "r1"}, {EntryID: "e1", RevisionID: "r2"}}} {
		s, p := pinService(t)
		c := pinCommand(t, s, p)
		c.Attachments = refs
		if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		assertNoPin(t, s, p, c)
	}
}

func TestDetectivePinFreshOwnerAuthorizationOnReplay(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	foreign := p
	foreign.Actor = "mallory"
	bindPin(t, foreign, &c)
	if _, err := s.PinDetectiveInput(context.Background(), foreign, c); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign private owner accepted: %v", err)
	}
	bindPin(t, p, &c)
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	s.authorize = func(_ context.Context, access Access) error {
		if access.Action == "pinDetectiveInput" {
			return ErrForbidden
		}
		return nil
	}
	if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrForbidden) {
		t.Fatalf("replay bypassed current authorization: %v", err)
	}
}

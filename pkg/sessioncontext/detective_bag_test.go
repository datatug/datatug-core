package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

func bagPinCommand(t *testing.T, s *Service, p Principal) PinDetectiveInputCommand {
	t.Helper()
	c := pinCommand(t, s, p)
	put := Command{OpID: "put-e1", Action: "putEntry", SessionID: "s1", ExpectedVersion: 1, BagID: "b1", ExpectedBagVersion: 1, EntryID: "e1", RevisionID: "r1", Scope: &c.Scope, Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "m1"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("111"))}}
	receipt := executeOK(t, s, p, put)
	apply := executeOK(t, s, p, Command{OpID: "apply", Action: "applyContext", SessionID: "s1", ExpectedVersion: receipt.SessionVersion, BagID: "b1", ExpectedBagVersion: 2, ExpectedContextSetID: c.ExpectedContextSetID, Scope: &c.Scope, Enabled: boolPtr(true), Selection: []EntryRef{{EntryID: "e1", RevisionID: "r1"}}})
	put.OpID = "put-e2"
	put.ExpectedVersion = apply.SessionVersion
	put.ExpectedBagVersion = 2
	put.EntryID = "e2"
	put.RevisionID = "r2"
	put.Value = &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("222"))}
	receipt = executeOK(t, s, p, put)
	snap, err := s.GetSnapshot(context.Background(), p, "s1")
	if err != nil {
		t.Fatal(err)
	}
	c.ExpectedVersion = receipt.SessionVersion
	c.ExpectedContextSetID = snap.ContextSet.ID
	c.Pin.Reference.ContextSetID = snap.ContextSet.ID
	c.Attachments = []EntryRef{{EntryID: "e2", RevisionID: "r2"}}
	for i, name := range []string{"first", "second"} {
		c.Pin.Definition.Check.Inputs = append(c.Pin.Definition.Check.Inputs, detective.Input{Name: name, Type: "integer", Cardinality: "scalar", Required: true})
		c.Pin.Reference.Bindings = append(c.Pin.Reference.Bindings, apicontract.SessionInputBindingRef{InputName: name, Origin: "bag", Entries: []apicontract.SessionInputEntryRef{{BagID: "b1", BagVersion: int64(i + 2), EntryID: []string{"e1", "e2"}[i], RevisionID: []string{"r1", "r2"}[i]}}})
		c.Pin.EffectiveInputs[name] = detective.InputValue{Scalar: valuePtr(apicontract.NewIntegerValue([]string{"111", "222"}[i]))}
	}
	bindPin(t, p, &c)
	return c
}
func TestDetectivePinPreservesMixedHistoricalBagVersions(t *testing.T) {
	s, p := pinService(t)
	c := bagPinCommand(t, s, p)
	accepted, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted.Input.Entries) != 2 || accepted.Input.Detective.Reference.Bindings[0].Entries[0].BagVersion != 2 || accepted.Input.Detective.Reference.Bindings[1].Entries[0].BagVersion != 3 {
		t.Fatal("historical bag provenance lost")
	}
	var work AcceptedDetectiveWork
	err = s.repo.View(context.Background(), func(r Reader) error { _, e := r.Load(context.Background(), DetectiveWork, "run", &work); return e })
	if err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(work)
	for _, private := range []string{"111", "222", "private_value", "Private Check"} {
		if strings.Contains(string(metadata), private) {
			t.Fatal("raw input copied to accepted work")
		}
	}
	c.Pin.Reference.Bindings[0].Entries[0].BagVersion = 99
	c.Pin.EffectiveInputs["first"].Scalar.Str = "999"
	c.Attachments[0].RevisionID = "changed"
	got, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if err != nil || got.Reference.Bindings[0].Entries[0].BagVersion != 2 || got.EffectiveInputs["first"].Scalar.Str != "111" {
		t.Fatalf("pin aliases bag caller: %v", err)
	}
}
func TestDetectivePinBagIdentityAndValueFailures(t *testing.T) {
	cases := map[string]func(*PinDetectiveInputCommand){
		"unselected":        func(c *PinDetectiveInputCommand) { c.Attachments = nil },
		"wrong-revision":    func(c *PinDetectiveInputCommand) { c.Pin.Reference.Bindings[1].Entries[0].RevisionID = "r1" },
		"wrong-bag":         func(c *PinDetectiveInputCommand) { c.Pin.Reference.Bindings[1].Entries[0].BagID = "other" },
		"wrong-bag-version": func(c *PinDetectiveInputCommand) { c.Pin.Reference.Bindings[1].Entries[0].BagVersion = 99 },
		"wrong-value":       func(c *PinDetectiveInputCommand) { c.Pin.EffectiveInputs["first"].Scalar.Str = "999" },
		"aggregation": func(c *PinDetectiveInputCommand) {
			c.Pin.Reference.Bindings[0].Entries = append(c.Pin.Reference.Bindings[0].Entries, c.Pin.Reference.Bindings[1].Entries[0])
		},
		"conflicting-revision": func(c *PinDetectiveInputCommand) {
			c.Attachments = append(c.Attachments, EntryRef{EntryID: "e1", RevisionID: "r2"})
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, p := pinService(t)
			c := bagPinCommand(t, s, p)
			change(&c)
			bindPin(t, p, &c)
			if _, err := s.PinDetectiveInput(context.Background(), p, c); err == nil {
				t.Fatal("invalid bag mapping accepted")
			}
			assertNoPin(t, s, p, c)
		})
	}
	for _, collection := range []string{Bags, Revisions} {
		t.Run("load-"+collection, func(t *testing.T) {
			s, p := pinService(t)
			c := bagPinCommand(t, s, p)
			base := s.repo
			s.repo = &interceptedRepository{Repository: base, failLoad: func(actual, _ string) error {
				if actual == collection {
					return ErrUnavailable
				}
				return nil
			}}
			if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
			s.repo = base
			assertNoPin(t, s, p, c)
		})
	}
}
func TestDetectivePinUnsupportedBagValuesDoNotChangeStoredContext(t *testing.T) {
	for _, value := range []ValueEnvelope{{SchemaVersion: 1, Kind: "set"}, {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "id", Value: apicontract.NewIntegerValue("1")}}}}}} {
		s, p := pinService(t)
		c := pinCommand(t, s, p)
		receipt := executeOK(t, s, p, Command{OpID: "unsupported-entry", Action: "putEntry", SessionID: "s1", ExpectedVersion: 1, BagID: "b1", ExpectedBagVersion: 1, EntryID: "e", RevisionID: "r", Scope: &c.Scope, Subject: &Subject{ConceptID: "Order", FieldID: "Order.ID", FieldMappingRef: "m"}, Value: &value})
		c.ExpectedVersion = receipt.SessionVersion
		c.Attachments = []EntryRef{{EntryID: "e", RevisionID: "r"}}
		c.Pin.Definition.Check.Inputs = []detective.Input{{Name: "ids", Type: "integer", Cardinality: "set", Required: true}}
		c.Pin.Reference.Bindings = []apicontract.SessionInputBindingRef{{InputName: "ids", Origin: "bag", Entries: []apicontract.SessionInputEntryRef{{BagID: "b1", BagVersion: 2, EntryID: "e", RevisionID: "r"}}}}
		set, _ := apicontract.NewTypedValueSet(apicontract.NewIntegerValue("1"))
		c.Pin.EffectiveInputs = map[string]detective.InputValue{"ids": {Set: &set}}
		bindPin(t, p, &c)
		_, err := s.PinDetectiveInput(context.Background(), p, c)
		if !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
		if len(value.Keys) == 0 && !errors.Is(err, ErrEmptyCheckSet) {
			t.Fatalf("empty Check set reason lost: %v", err)
		}
		assertNoPin(t, s, p, c)
		revision, err := s.ReadEntryRevision(context.Background(), p, "s1", "e", "r")
		if err != nil || len(revision.Value.Values) != 0 || len(revision.Value.Keys) != len(value.Keys) {
			t.Fatalf("bag meaning changed: %v", err)
		}
	}
	set, _ := apicontract.NewTypedValueSet(apicontract.NewIntegerValue("1"), apicontract.NewIntegerValue("2"))
	got, err := detectiveBagValue(ValueEnvelope{SchemaVersion: 1, Kind: "set", Values: set.Values})
	if err != nil || got.Set == nil || len(got.Set.Values) != 2 {
		t.Fatalf("nonempty set rejected: %v", err)
	}
	if _, err = detectiveBagValue(ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: &apicontract.TypedValue{Type: "bad"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

package sessioncontext

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

var detectiveWorkDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

var detectiveWorkPrivatePayload = []string{"111", "222", "private_value", "Private Check"}

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
	assertDetectiveWorkPrivacy(t, work)
	c.Pin.Reference.Bindings[0].Entries[0].BagVersion = 99
	c.Pin.EffectiveInputs["first"].Scalar.Str = "999"
	c.Attachments[0].RevisionID = "changed"
	got, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if err != nil || got.Reference.Bindings[0].Entries[0].BagVersion != 2 || got.EffectiveInputs["first"].Scalar.Str != "111" {
		t.Fatalf("pin aliases bag caller: %v", err)
	}
}

// detectiveWorkLeaks returns each accepted-work metadata path whose scalar
// value contains a raw pin payload. The typed acceptedAt timestamp and digest
// leaves are opaque metadata: legitimate RFC3339Nano timestamps and SHA-256
// spellings can contain the short numeric sentinels by coincidence, so they
// are validated by shape instead of substring-scanned. Every other scalar
// leaf, including identifiers, state and version counters, is scanned
// verbatim; this is not a general value allowlist.
func detectiveWorkLeaks(t *testing.T, work AcceptedDetectiveWork) []string {
	t.Helper()
	var leaks []string
	scan := func(path, value string) {
		for _, payload := range detectiveWorkPrivatePayload {
			if strings.Contains(value, payload) {
				leaks = append(leaks, path+" contains "+payload)
			}
		}
	}
	var walk func(path string, value reflect.Value)
	walk = func(path string, value reflect.Value) {
		if !value.IsValid() {
			return
		}
		switch value.Kind() {
		case reflect.Pointer, reflect.Interface:
			if value.IsNil() {
				return
			}
			walk(path, value.Elem())
		case reflect.Struct:
			typ := value.Type()
			for index := 0; index < value.NumField(); index++ {
				field := typ.Field(index)
				if field.PkgPath != "" {
					continue
				}
				fieldPath := path + "." + field.Name
				fieldValue := value.Field(index)
				if field.Type == reflect.TypeOf(time.Time{}) {
					if fieldValue.Interface().(time.Time).IsZero() {
						leaks = append(leaks, fieldPath+" is not a set timestamp")
					}
					continue
				}
				if field.Type.Kind() == reflect.String && strings.HasSuffix(field.Name, "Digest") {
					if digest := fieldValue.String(); digest != "" && !detectiveWorkDigest.MatchString(digest) {
						leaks = append(leaks, fieldPath+" is not a canonical digest")
					}
					continue
				}
				walk(fieldPath, fieldValue)
			}
		case reflect.Slice, reflect.Array:
			for index := 0; index < value.Len(); index++ {
				walk(fmt.Sprintf("%s[%d]", path, index), value.Index(index))
			}
		case reflect.Map:
			iter := value.MapRange()
			for iter.Next() {
				walk(fmt.Sprintf("%s[%v]", path, iter.Key().Interface()), iter.Value())
			}
		default:
			scan(path, fmt.Sprint(value.Interface()))
		}
	}
	walk("AcceptedDetectiveWork", reflect.ValueOf(work))
	return leaks
}

func assertDetectiveWorkPrivacy(t *testing.T, work AcceptedDetectiveWork) {
	t.Helper()
	if leaks := detectiveWorkLeaks(t, work); len(leaks) != 0 {
		t.Fatalf("raw input copied to accepted work: %s", strings.Join(leaks, "; "))
	}
}

func TestDetectiveWorkPrivacyBoundary(t *testing.T) {
	valid := AcceptedDetectiveWork{
		ID:              "run",
		Owner:           "alice",
		SessionID:       "s1",
		RunID:           "run",
		InputSnapshotID: "input",
		State:           "accepted",
		Version:         1,
		AcceptedAt:      time.Date(2026, 10, 26, 17, 48, 51, 111222333, time.UTC),
	}
	valid.Reference.DefinitionDigest = strings.Repeat("1", 64)
	valid.Dispatch.DefinitionDigest = strings.Repeat("1", 64)
	valid.Dispatch.NativeRequestDigest = strings.Repeat("2", 64)
	valid.AcceptanceDigest = strings.Repeat("2", 64)
	if leaks := detectiveWorkLeaks(t, valid); len(leaks) != 0 {
		t.Fatalf("legal timestamp/digest metadata rejected: %v", leaks)
	}

	cases := []struct {
		name   string
		inject func(*AcceptedDetectiveWork)
		want   string
	}{
		{"input value in identifier", func(w *AcceptedDetectiveWork) { w.Reference.InputSnapshotID = "111" }, "111"},
		{"input value in state", func(w *AcceptedDetectiveWork) { w.State = "222" }, "222"},
		{"check title in owner", func(w *AcceptedDetectiveWork) { w.Owner = "Private Check" }, "Private Check"},
		{"query payload in dispatch", func(w *AcceptedDetectiveWork) { w.Dispatch.CheckID = "select private_value" }, "private_value"},
		{"input value disguised as digest", func(w *AcceptedDetectiveWork) { w.Reference.DefinitionDigest = "111" }, "not a canonical digest"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			injected := valid
			test.inject(&injected)
			leaks := detectiveWorkLeaks(t, injected)
			if len(leaks) == 0 || !strings.Contains(strings.Join(leaks, "; "), test.want) {
				t.Fatalf("payload %q was not reported: %v", test.want, leaks)
			}
		})
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

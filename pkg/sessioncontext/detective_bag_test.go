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

var detectiveWorkTimeType = reflect.TypeOf(time.Time{})

// detectiveTestClock is the deterministic acceptance clock installed by
// pinWithTestClock. Privacy expectations bind AcceptedAt to exactly this known
// value instead of reading the stored work back as its own expectation.
var detectiveTestClock = time.Date(2026, 10, 26, 17, 48, 51, 111222333, time.UTC)

// The only accepted-work leaves allowed to carry opaque metadata are named
// explicitly. A field name suffix, a 64-hex shape, or a same-named nested
// field never exempts a leaf: an authorized path must match its independently
// expected value exactly, and every other scalar leaf is scanned for raw
// payloads. A disguised digest- or timestamp-shaped path is reported.
const (
	detectiveWorkReferenceDefinitionDigest = "AcceptedDetectiveWork.Reference.DefinitionDigest"
	detectiveWorkDispatchDefinitionDigest  = "AcceptedDetectiveWork.Dispatch.DefinitionDigest"
	detectiveWorkNativeRequestDigest       = "AcceptedDetectiveWork.Dispatch.NativeRequestDigest"
	detectiveWorkCaseContextDigest         = "AcceptedDetectiveWork.Dispatch.CaseContextDigest"
	detectiveWorkAcceptanceDigest          = "AcceptedDetectiveWork.AcceptanceDigest"
	detectiveWorkClientRequestDigest       = "AcceptedDetectiveWork.ClientRequestDigest"
	detectiveWorkAcceptedAtPath            = "AcceptedDetectiveWork.AcceptedAt"
)

// detectiveWorkBinding is the trusted expectation a privacy scan is checked
// against: exact digest values per authorized path and the known acceptance
// clock. It never derives an expectation from the value under test.
type detectiveWorkBinding struct {
	digests    map[string]string
	acceptedAt time.Time
}

func detectiveWorkDigestPath(path string) bool {
	switch path {
	case detectiveWorkReferenceDefinitionDigest, detectiveWorkDispatchDefinitionDigest,
		detectiveWorkNativeRequestDigest, detectiveWorkCaseContextDigest,
		detectiveWorkAcceptanceDigest, detectiveWorkClientRequestDigest:
		return true
	}
	return false
}

func pinWithTestClock(t *testing.T) (*Service, Principal) {
	t.Helper()
	s, p := pinService(t)
	s.now = func() time.Time { return detectiveTestClock }
	return s, p
}

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

// bagWorkBinding derives privacy expectations from the authorized pin command
// and the acknowledgment it returned, plus the known acceptance clock. None of
// the expected digests is read from the stored accepted work under test.
func bagWorkBinding(t *testing.T, c PinDetectiveInputCommand, accepted PinnedDetectiveInput) detectiveWorkBinding {
	t.Helper()
	if _, digest, err := CanonicalDetectiveDefinition(context.Background(), c.Pin.Definition); err != nil || digest != c.Pin.Reference.DefinitionDigest || digest != c.Pin.Dispatch.DefinitionDigest {
		t.Fatalf("authorized definition digest fixture: %v", err)
	}
	if c.NativeBinding.PayloadDigest == "" || c.NativeBinding.PayloadDigest != c.Pin.Dispatch.NativeRequestDigest {
		t.Fatal("authorized native request digest fixture is not bound")
	}
	if accepted.Receipt.PayloadDigest == "" {
		t.Fatal("acknowledged acceptance digest is missing")
	}
	return detectiveWorkBinding{
		digests: map[string]string{
			detectiveWorkReferenceDefinitionDigest: c.Pin.Reference.DefinitionDigest,
			detectiveWorkDispatchDefinitionDigest:  c.Pin.Dispatch.DefinitionDigest,
			detectiveWorkNativeRequestDigest:       c.NativeBinding.PayloadDigest,
			detectiveWorkCaseContextDigest:         c.Pin.Dispatch.CaseContextDigest,
			detectiveWorkAcceptanceDigest:          accepted.Receipt.PayloadDigest,
			detectiveWorkClientRequestDigest:       c.ClientRequestDigest,
		},
		acceptedAt: detectiveTestClock,
	}
}

func TestDetectivePinPreservesMixedHistoricalBagVersions(t *testing.T) {
	s, p := pinWithTestClock(t)
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
	assertDetectiveWorkPrivacy(t, work, bagWorkBinding(t, c, accepted))
	c.Pin.Reference.Bindings[0].Entries[0].BagVersion = 99
	c.Pin.EffectiveInputs["first"].Scalar.Str = "999"
	c.Attachments[0].RevisionID = "changed"
	got, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if err != nil || got.Reference.Bindings[0].Entries[0].BagVersion != 2 || got.EffectiveInputs["first"].Scalar.Str != "111" {
		t.Fatalf("pin aliases bag caller: %v", err)
	}
}

// detectiveWorkLeaks returns each accepted-work leaf that copies a raw pin
// payload or that fails its trusted metadata binding. Only the explicitly
// named digest paths may carry an opaque digest, and only when the value is
// exactly the independently expected one; only AcceptedAt may carry a
// timestamp, and only when it equals the known acceptance clock. Every other
// scalar leaf, including identifiers, state, counters and any nested or
// disguised digest/timestamp field, is scanned verbatim. This is a precise
// path binding, not a value or shape allowlist.
func detectiveWorkLeaks(t *testing.T, work any, expected detectiveWorkBinding) []string {
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
				if fieldValue.Type() == detectiveWorkTimeType {
					stamp := fieldValue.Interface().(time.Time)
					if fieldPath != detectiveWorkAcceptedAtPath {
						leaks = append(leaks, fieldPath+" is not an authorized timestamp path")
						scan(fieldPath, fmt.Sprint(stamp))
						continue
					}
					if stamp.IsZero() {
						leaks = append(leaks, fieldPath+" is not a set timestamp")
					} else if !stamp.Equal(expected.acceptedAt) {
						leaks = append(leaks, fieldPath+" is not the expected accepted timestamp")
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
		case reflect.String:
			leaf := value.String()
			if detectiveWorkDigestPath(path) {
				bound, ok := expected.digests[path]
				switch {
				case !ok:
					leaks = append(leaks, path+" has no independently expected binding")
				case leaf != bound:
					leaks = append(leaks, path+" is not the expected digest binding")
				case leaf != "" && !detectiveWorkDigest.MatchString(leaf):
					leaks = append(leaks, path+" is not a canonical digest")
				}
				return
			}
			if name := path[strings.LastIndex(path, ".")+1:]; strings.HasSuffix(name, "Digest") || name == "AcceptedAt" {
				leaks = append(leaks, path+" is not an authorized metadata path")
			}
			scan(path, leaf)
		default:
			scan(path, fmt.Sprint(value.Interface()))
		}
	}
	walk("AcceptedDetectiveWork", reflect.ValueOf(work))
	return leaks
}

func assertDetectiveWorkPrivacy(t *testing.T, work AcceptedDetectiveWork, expected detectiveWorkBinding) {
	t.Helper()
	if leaks := detectiveWorkLeaks(t, work, expected); len(leaks) != 0 {
		t.Fatalf("raw input copied to accepted work: %s", strings.Join(leaks, "; "))
	}
}

// Controlled trusted expectations for the unit privacy fixture. Every value
// carries the 111/222 numeric sentinels on purpose and is distinct per path, so
// acceptance proves the exemption is the exact expected binding for that path,
// never a suffix, a 64-hex shape, or a substring heuristic.
var (
	detectiveFixtureReferenceDigest  = strings.Repeat("1", 32) + strings.Repeat("2", 32)
	detectiveFixtureDispatchDigest   = strings.Repeat("2", 32) + strings.Repeat("1", 32)
	detectiveFixtureNativeDigest     = strings.Repeat("1", 64)
	detectiveFixtureAcceptanceDigest = strings.Repeat("2", 64)
)

func validDetectiveWork() AcceptedDetectiveWork {
	valid := AcceptedDetectiveWork{
		ID:              "run",
		Owner:           "alice",
		SessionID:       "s1",
		RunID:           "run",
		InputSnapshotID: "input",
		State:           "accepted",
		Version:         1,
		AcceptedAt:      detectiveTestClock,
	}
	valid.Reference.DefinitionDigest = detectiveFixtureReferenceDigest
	valid.Dispatch.DefinitionDigest = detectiveFixtureDispatchDigest
	valid.Dispatch.NativeRequestDigest = detectiveFixtureNativeDigest
	valid.AcceptanceDigest = detectiveFixtureAcceptanceDigest
	return valid
}

func validDetectiveWorkBinding() detectiveWorkBinding {
	return detectiveWorkBinding{
		digests: map[string]string{
			detectiveWorkReferenceDefinitionDigest: detectiveFixtureReferenceDigest,
			detectiveWorkDispatchDefinitionDigest:  detectiveFixtureDispatchDigest,
			detectiveWorkNativeRequestDigest:       detectiveFixtureNativeDigest,
			detectiveWorkCaseContextDigest:         "",
			detectiveWorkAcceptanceDigest:          detectiveFixtureAcceptanceDigest,
			detectiveWorkClientRequestDigest:       "",
		},
		acceptedAt: detectiveTestClock,
	}
}

func TestDetectiveWorkPrivacyBoundary(t *testing.T) {
	valid := validDetectiveWork()
	if leaks := detectiveWorkLeaks(t, valid, validDetectiveWorkBinding()); len(leaks) != 0 {
		t.Fatalf("exactly bound timestamp/digest metadata rejected: %v", leaks)
	}

	cases := []struct {
		name   string
		mutate func(*AcceptedDetectiveWork, *detectiveWorkBinding)
		want   string
	}{
		{"input value in identifier", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) { w.Reference.InputSnapshotID = "111" }, "contains 111"},
		{"input value in state", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) { w.State = "222" }, "contains 222"},
		{"check title in owner", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) { w.Owner = "Private Check" }, "contains Private Check"},
		{"query payload in dispatch", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) { w.Dispatch.CheckID = "select private_value" }, "contains private_value"},
		{"value leak in nested binding name", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) {
			w.Reference.Bindings = []apicontract.SessionInputBindingRef{{InputName: "private_value", Origin: "manual"}}
		}, "contains private_value"},
		{"valid raw digest bound to another expectation", func(_ *AcceptedDetectiveWork, b *detectiveWorkBinding) {
			b.digests[detectiveWorkReferenceDefinitionDigest] = strings.Repeat("3", 64)
		}, "not the expected digest binding"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			injected := valid
			expected := validDetectiveWorkBinding()
			test.mutate(&injected, &expected)
			leaks := detectiveWorkLeaks(t, injected, expected)
			if len(leaks) == 0 || !strings.Contains(strings.Join(leaks, "; "), test.want) {
				t.Fatalf("payload %q was not reported: %v", test.want, leaks)
			}
		})
	}
}

func TestDetectiveWorkPrivacyRejectsMissingOrWrongBinding(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*AcceptedDetectiveWork, *detectiveWorkBinding)
		want   string
	}{
		{"digest without expected binding", func(_ *AcceptedDetectiveWork, b *detectiveWorkBinding) {
			delete(b.digests, detectiveWorkNativeRequestDigest)
		}, "has no independently expected binding"},
		{"substituted valid digest", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) {
			w.Reference.DefinitionDigest = strings.Repeat("2", 64)
		}, "is not the expected digest binding"},
		{"zero accepted timestamp", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) {
			w.AcceptedAt = time.Time{}
		}, "is not a set timestamp"},
		{"unexpected accepted timestamp", func(w *AcceptedDetectiveWork, _ *detectiveWorkBinding) {
			w.AcceptedAt = detectiveTestClock.Add(time.Second)
		}, "is not the expected accepted timestamp"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			work := validDetectiveWork()
			expected := validDetectiveWorkBinding()
			test.mutate(&work, &expected)
			leaks := detectiveWorkLeaks(t, work, expected)
			if len(leaks) == 0 || !strings.Contains(strings.Join(leaks, "; "), test.want) {
				t.Fatalf("binding failure %q was not reported: %v", test.want, leaks)
			}
		})
	}
}

func TestDetectiveWorkPrivacyRejectsDisguisedMetadataPaths(t *testing.T) {
	forged := struct {
		AcceptedAt time.Time
		Forged     struct {
			DefinitionDigest string
			AcceptedAt       time.Time
		}
	}{AcceptedAt: detectiveTestClock}
	forged.Forged.DefinitionDigest = strings.Repeat("1", 64)
	forged.Forged.AcceptedAt = detectiveTestClock
	leaks := detectiveWorkLeaks(t, forged, validDetectiveWorkBinding())
	joined := strings.Join(leaks, "; ")
	for _, want := range []string{
		"AcceptedDetectiveWork.Forged.DefinitionDigest is not an authorized metadata path",
		"AcceptedDetectiveWork.Forged.AcceptedAt is not an authorized timestamp path",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("disguised metadata path %q was not refused: %v", want, leaks)
		}
	}
	if strings.Contains(joined, detectiveWorkAcceptedAtPath) {
		t.Fatalf("authorized timestamp path was refused: %v", leaks)
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

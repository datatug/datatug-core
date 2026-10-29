package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"testing"
)

// Cancel at a deterministic read checkpoint, using a real canceled context.
// Each iteration proves cancellation discards a partially accumulated result.
type retainedCancelCheckpoint struct {
	context.Context
	cancel context.CancelFunc
	left   int
}

func (c *retainedCancelCheckpoint) Err() error {
	c.left--
	if c.left == 0 {
		c.cancel()
	}
	return c.Context.Err()
}
func checkpointContext(n int) *retainedCancelCheckpoint {
	ctx, cancel := context.WithCancel(context.Background())
	return &retainedCancelCheckpoint{ctx, cancel, n}
}

func mutateRetainedBody(t *testing.T, inventory RetainedInventory, collection, id string, change func(map[string]any)) RetainedInventory {
	t.Helper()
	if id == "" {
		for _, d := range inventory.Documents {
			if d.Collection == collection {
				id = d.ID
				break
			}
		}
	}
	return mutateRetainedContext(t, inventory, collection, id, func(d *RetainedDocument) {
		var body map[string]any
		if err := json.Unmarshal(d.Payload, &body); err != nil {
			t.Fatal(err)
		}
		change(body)
		d.Payload, _ = json.Marshal(body)
	})
}
func retainedKeys(r retainedInventoryReader) []string {
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestRetainedProjectionCancellationAtEveryReadCheckpoint(t *testing.T) {
	inv, _, _, _, _, _, _ := retainedDetectiveFixture(t, true)
	contextInv, _ := retainedContextFixture(t)
	for _, fixture := range []RetainedInventory{inv, contextInv, retainedOrdinaryFixture(t), retainedHistoryFixture(t)} {
		for _, project := range []func(context.Context, RetainedInventory) error{
			func(ctx context.Context, i RetainedInventory) error {
				out, err := ProjectRetainedContextBindings(ctx, i)
				if err != nil && out != nil {
					t.Fatal("partial contexts on cancellation")
				}
				return err
			},
			func(ctx context.Context, i RetainedInventory) error {
				out, err := ProjectRetainedDetectiveExecutions(ctx, i)
				if err != nil && out != nil {
					t.Fatal("partial native executions on cancellation")
				}
				return err
			},
			func(ctx context.Context, i RetainedInventory) error {
				out, err := ProjectRetainedSessionSources(ctx, i)
				if err != nil && !reflect.DeepEqual(out, RetainedSessionSources{}) {
					t.Fatal("partial source inventory on cancellation")
				}
				return err
			},
		} {
			for n := 1; n < 500; n++ {
				ctx := checkpointContext(n)
				err := project(ctx, fixture)
				canceled := ctx.Context.Err() != nil
				ctx.cancel()
				if canceled && !errors.Is(err, context.Canceled) {
					t.Fatalf("checkpoint %d lost cancellation: %v", n, err)
				}
				if !canceled {
					break
				}
			}
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, testCase := range []struct {
		name string
		ctx  context.Context
		want error
	}{
		{name: "nil context", ctx: nil, want: ErrInvalid},
		{name: "canceled context", ctx: ctx, want: context.Canceled},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if out, err := ProjectRetainedSessionSources(testCase.ctx, inv); !errors.Is(err, testCase.want) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
				t.Fatal("invalid context accepted or partial sources returned", err)
			}
			if out, err := ProjectRetainedDetectiveExecutions(testCase.ctx, inv); !errors.Is(err, testCase.want) || out != nil {
				t.Fatal("invalid context accepted or partial native output returned", err)
			}
		})
	}
	r, _ := retainedReader(inv)
	if _, err := r.Load(ctx, Sessions, "s1", &Session{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if out, next, err := r.List(context.Background(), Sessions, "", "", 1); out != nil || next != "" || !errors.Is(err, ErrUnavailable) {
		t.Fatal("internal reader exposed filtered partial inventory")
	}
}

func TestRetainedContextRejectsEachBrokenGraphBoundary(t *testing.T) {
	inv, _ := retainedContextFixture(t)
	cases := []struct {
		name, collection, id, field string
		value                       any
	}{
		{"session schema", Sessions, "session", "schemaVersion", 2},
		{"session state", Sessions, "session", "state", "future"},
		{"duplicate bags", Sessions, "session", "bagIds", []string{"bag", "bag"}},
		{"default missing", Sessions, "session", "defaultBagId", "not-declared"},
		{"bag schema", Bags, "bag", "clearVersion", -1},
		{"bag duplicate", Bags, "bag", "entryIds", []string{"entry", "entry"}},
		{"entry schema", Entries, "entry", "currentRevisionId", "bad/id"},
		{"revision empty provenance", Revisions, "revision-a", "sourceDigest", "unexpected"},
		{"revision copy missing source", Revisions, "revision-a", "provenance", "work-context-copy"},
		{"revision legacy missing source", Revisions, "revision-a", "provenance", "legacy-import"},
		{"revision unknown provenance", Revisions, "revision-a", "provenance", "future"},
		{"set schema", ContextSets, "", "createdAt", "0001-01-01T00:00:00Z"},
		{"missing current set", Sessions, "session", "contextSetId", "missing"},
		{"missing session bag", Sessions, "session", "bagIds", []string{"bag", "missing"}},
		{"missing bag entry", Bags, "bag", "entryIds", []string{"missing"}},
		{"missing entry bag", Entries, "entry", "bagId", "missing"},
		{"revision head mismatch", Revisions, "revision-b", "entryId", "different"},
		{"missing revision entry", Revisions, "revision-a", "entryId", "missing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bad := mutateRetainedBody(t, inv, c.collection, c.id, func(b map[string]any) { b[c.field] = c.value })
			if out, err := ProjectRetainedContextBindings(context.Background(), bad); !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatalf("invalid graph yielded partial output: %v %v", out, err)
			}
		})
	}
	// Slot corruption must be tested on a retained set with an actual selection.
	setID := ""
	for _, d := range inv.Documents {
		if d.Collection == ContextSets {
			var set AppliedContextSet
			_ = json.Unmarshal(d.Payload, &set)
			if len(set.Slots) > 0 && len(set.Slots[0].Entries) > 0 {
				setID = d.ID
				break
			}
		}
	}
	if setID == "" {
		t.Fatal("missing selected set")
	}
	for _, mode := range []string{"duplicate scope", "invalid empty bag", "missing bag", "missing selected entry"} {
		t.Run(mode, func(t *testing.T) {
			bad := mutateRetainedContext(t, inv, ContextSets, setID, func(d *RetainedDocument) {
				var s AppliedContextSet
				_ = json.Unmarshal(d.Payload, &s)
				switch mode {
				case "duplicate scope":
					s.Slots = append(s.Slots, s.Slots[0])
				case "invalid empty bag":
					s.Slots[0].BagID = ""
				case "missing bag":
					s.Slots[0].BagID = "missing"
				case "missing selected entry":
					s.Slots[0].Entries[0].EntryID = "missing"
				}
				d.Payload, _ = json.Marshal(s)
			})
			if out, err := ProjectRetainedContextBindings(context.Background(), bad); !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatal("broken selection accepted", err)
			}
		})
	}
	bad := inv
	bad.Collections = append(append([]string(nil), inv.Collections...), Sessions)
	if out, err := ProjectRetainedContextBindings(context.Background(), bad); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("duplicate manifest accepted")
	}
	bad = inv
	bad.Documents = append(append([]RetainedDocument(nil), inv.Documents...), inv.Documents[0])
	if _, err := retainedReader(bad); !errors.Is(err, ErrUnavailable) {
		t.Fatal("duplicate retained identity accepted")
	}
}

func TestRetainedInputSubjectJoinRejectsMissingForeignAndRepeatedResources(t *testing.T) {
	inv, _ := retainedContextFixture(t)
	r, _ := retainedReader(inv)
	input := InputSnapshot{ID: "input", SessionID: "session", Scope: Scope{"uat", "tenant-a"}, Entries: []EntryRef{{EntryID: "entry", RevisionID: "revision-a"}}}
	r[Inputs+"/input"] = RetainedDocument{Owner: "alice"}
	for _, mode := range []string{"invalid id", "duplicate", "entry missing", "revision missing", "bag missing", "foreign owner", "subject quota"} {
		t.Run(mode, func(t *testing.T) {
			copy := retainedInventoryReader{}
			for k, d := range r {
				copy[k] = d
			}
			in := input
			in.Entries = append([]EntryRef(nil), input.Entries...)
			budget := &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000}
			switch mode {
			case "invalid id":
				in.Entries[0].EntryID = "bad/id"
			case "duplicate":
				in.Entries = append(in.Entries, in.Entries[0])
			case "entry missing":
				delete(copy, Entries+"/entry")
			case "revision missing":
				delete(copy, Revisions+"/revision-a")
			case "bag missing":
				delete(copy, Bags+"/bag")
			case "foreign owner":
				d := copy[Bags+"/bag"]
				d.Owner = "bob"
				copy[Bags+"/bag"] = d
			case "subject quota":
				budget.subjects = 0
			}
			if out, err := retainedInputSubjects(context.Background(), copy, in, budget); !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatal("invalid subject join yielded output", err)
			}
		})
	}
	for n := 1; n < 20; n++ {
		ctx := checkpointContext(n)
		out, err := retainedInputSubjects(ctx, r, input, &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000})
		canceled := ctx.Context.Err() != nil
		ctx.cancel()
		if canceled && (!errors.Is(err, context.Canceled) && !errors.Is(err, ErrUnavailable) || out != nil) {
			t.Fatal("canceled subject join exposed output", err)
		}
		if !canceled {
			break
		}
	}
	if (&retainedSessionSourceBudget{bytes: 100, subjects: 1}).charge(make(chan int), 0) {
		t.Fatal("unencodable source consumed budget successfully")
	}
}

func TestRetainedProjectionDependencyFailuresNeverReturnPartialResults(t *testing.T) {
	ordinary, _ := retainedContextFixture(t)
	native, _, _, _, _, _, _ := retainedDetectiveFixture(t, true)
	oldDecode, oldMarshal, oldIdentity := decodeRetainedSource, marshalRetainedSource, retainedLifecycleIdentity
	t.Cleanup(func() {
		decodeRetainedSource, marshalRetainedSource, retainedLifecycleIdentity = oldDecode, oldMarshal, oldIdentity
	})
	failure := errors.New("source dependency unavailable")
	for _, inv := range []RetainedInventory{ordinary, native, retainedOrdinaryFixture(t), retainedHistoryFixture(t)} {
		for _, boundary := range []string{"decode", "marshal", "identity"} {
			for failAt := 1; failAt < 1000; failAt++ {
				decodeRetainedSource, marshalRetainedSource, retainedLifecycleIdentity = oldDecode, oldMarshal, oldIdentity
				calls, hit := 0, false
				fault := func() bool {
					calls++
					if calls == failAt {
						hit = true
						return true
					}
					return false
				}
				switch boundary {
				case "decode":
					decodeRetainedSource = func(body []byte, out any, forbidden ...string) error {
						if fault() {
							return failure
						}
						return oldDecode(body, out, forbidden...)
					}
				case "marshal":
					marshalRetainedSource = func(v any) ([]byte, error) {
						if fault() {
							return nil, failure
						}
						return oldMarshal(v)
					}
				case "identity":
					retainedLifecycleIdentity = func(n NativeNamespace, id string) (string, error) {
						if fault() {
							return "", failure
						}
						return oldIdentity(n, id)
					}
				}
				out, err := ProjectRetainedSessionSources(context.Background(), inv)
				if hit && (err == nil || !reflect.DeepEqual(out, RetainedSessionSources{})) {
					t.Fatalf("%s failure at dependency %d exposed partial source output", boundary, failAt)
				}
				if !hit {
					if err != nil {
						t.Fatal("restored source dependency did not recover", err)
					}
					break
				}
			}
		}
	}
}

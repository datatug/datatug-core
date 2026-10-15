package sessioncontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

func pinService(t *testing.T) (*Service, Principal) {
	t.Helper()
	repo, err := NewNativePinRepository(testMountedDatabase{"db1", dalgo2memory.New(dalgo2memory.FirestoreProfile())}, nil, "local", "", "project1")
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewNativePinService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	return svc, Principal{Actor: "alice", DatabaseID: "db1", ProjectID: "project1", Executor: true}
}
func pinCommand(t *testing.T, s *Service, p Principal) PinDetectiveInputCommand {
	t.Helper()
	receipt := createTestSession(t, s, p, "s1", "b1")
	c := PinDetectiveInputCommand{OpID: "pin", SessionID: "s1", InputSnapshotID: "input", RunID: "run", ExpectedVersion: receipt.SessionVersion, ExpectedContextSetID: "cs_" + opKey(p, "create-s1"), Scope: Scope{"uat", "tenant-a"}}
	n := *s.nativeNamespace
	c.Pin.Reference = apicontract.SessionInputReference{SchemaVersion: 1, StorageID: n.StorageID, SpaceID: n.SpaceID, DatabaseID: n.DatabaseID, ProjectID: n.ProjectID, NamespaceID: n.NamespaceID, SessionID: c.SessionID, InputSnapshotID: c.InputSnapshotID, ContextSetID: c.ExpectedContextSetID, EnvironmentID: c.Scope.EnvironmentID, IdentityDomain: c.Scope.IdentityDomain}
	c.Pin.Definition = DetectiveCheckDefinitionPin{SchemaVersion: 1, Check: detective.Check{ID: "check", Title: "Private Check title", Version: 1, Steps: []detective.Step{{ID: "q", Kind: "query", QueryID: "q"}}}, Queries: []DetectiveAcceptedQueryPin{{QueryID: "q", Revision: "q-r1", DefinitionJSON: json.RawMessage(`{"id":"q","title":"Private query title","type":"SQL","text":"select private_value"}`)}}}
	c.Pin.EffectiveInputs = map[string]detective.InputValue{}
	c.Pin.Dispatch = DetectiveDispatchBinding{NativeOperationID: "native-op", NativeStoreID: "local", NativeProjectID: "native-project", Action: "run-check", CheckID: "check", CheckVersion: 1}
	bindPin(t, p, &c)
	return c
}
func bindPin(t *testing.T, p Principal, c *PinDetectiveInputCommand) {
	t.Helper()
	_, digest, err := CanonicalDetectiveDefinition(context.Background(), c.Pin.Definition)
	if err != nil {
		t.Fatal(err)
	}
	c.Pin.Reference.DefinitionDigest = digest
	c.Pin.Dispatch.DefinitionDigest = digest
	d := &c.Pin.Dispatch
	if c.Case != nil {
		d.CaseID = c.Case.ID
		d.CaseVersion = c.Case.Version
		values := c.Case.Context
		if values == nil {
			values = map[string]detective.InputValue{}
		}
		raw, err := json.Marshal(values)
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		d.CaseContextDigest = hex.EncodeToString(h[:])
	}
	payload, err := detective.ActionOperationPayload(detective.ActionRequest{Type: d.Action, Project: d.NativeProjectID, CheckID: d.CheckID, CaseID: d.CaseID, Environment: c.Scope.EnvironmentID, Inputs: c.Pin.EffectiveInputs, InputReference: &c.Pin.Reference, ExpectedDefinitionDigest: d.DefinitionDigest, ExpectedCaseContextDigest: d.CaseContextDigest})
	if err != nil {
		t.Fatal(err)
	}
	c.NativeBinding, err = detective.BindOperation(d.NativeProjectID, p.Actor, d.Action, d.CheckID, payload)
	if err != nil {
		t.Fatal(err)
	}
	d.NativeRequestDigest = c.NativeBinding.PayloadDigest
}
func clonePinCommand(t *testing.T, c PinDetectiveInputCommand) PinDetectiveInputCommand {
	t.Helper()
	raw, err := json.Marshal(c.Pin)
	if err != nil {
		t.Fatal(err)
	}
	c.Pin = DetectiveInputPin{}
	if err = json.Unmarshal(raw, &c.Pin); err != nil {
		t.Fatal(err)
	}
	return c
}
func assertNoPin(t *testing.T, s *Service, p Principal, c PinDetectiveInputCommand) {
	t.Helper()
	err := s.repo.View(context.Background(), func(r Reader) error {
		for _, key := range []struct{ collection, id string }{{Inputs, c.InputSnapshotID}, {Runs, c.RunID}, {DetectiveWork, c.RunID}, {Operations, opKey(p, c.OpID)}, {Audit, opKey(p, c.OpID)}} {
			var v any
			if _, err := r.Load(context.Background(), key.collection, key.id, &v); !errors.Is(err, ErrNotFound) {
				t.Fatalf("partial record %s: %v", key.collection, err)
			}
		}
		var session Session
		if _, err := r.Load(context.Background(), Sessions, c.SessionID, &session); err != nil {
			return err
		}
		if session.PendingRunCount != 0 || session.Version != c.ExpectedVersion {
			t.Fatalf("partial session: %+v", session)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestDetectivePinAtomicReplayAndContentOwnership(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	first, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.PinDetectiveInput(context.Background(), p, c)
	a, _ := json.Marshal(first)
	b, _ := json.Marshal(retry)
	if err != nil || string(a) != string(b) {
		t.Fatalf("lost acknowledgment replay differs: %v", err)
	}
	var work AcceptedDetectiveWork
	if err = s.repo.View(context.Background(), func(r Reader) error { _, e := r.Load(context.Background(), DetectiveWork, c.RunID, &work); return e }); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal([]any{work, first.Receipt, first.Run})
	for _, secret := range []string{"private_value", "Private query title", "Private Check title"} {
		if strings.Contains(string(metadata), secret) {
			t.Fatalf("private content leaked into metadata: %s", secret)
		}
	}
	c.Pin.Definition.Queries[0].DefinitionJSON[0] = '!'
	c.Pin.Definition.Check.Title = "changed"
	first.Input.Detective.Definition.Check.Title = "changed result"
	got, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if err != nil || got.Definition.Check.Title != "Private Check title" || !strings.Contains(string(got.Definition.Queries[0].DefinitionJSON), "private_value") {
		t.Fatalf("owned pin changed: %v", err)
	}
	got.Definition.Check.Title = "mutated read"
	again, _ := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if again.Definition.Check.Title == got.Definition.Check.Title {
		t.Fatal("read aliases persisted payload")
	}
	session, _ := s.GetSnapshot(context.Background(), p, "s1")
	if session.Session.PendingRunCount != 1 {
		t.Fatal("replay incremented pending count")
	}
	changed := clonePinCommand(t, retryCommand(t, s, p, retry))
	changed.Pin.Definition.Queries[0].Revision = "q-r2"
	bindPin(t, p, &changed)
	if _, err = s.PinDetectiveInput(context.Background(), p, changed); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed replay: %v", err)
	}
}
func retryCommand(t *testing.T, s *Service, p Principal, v PinnedDetectiveInput) PinDetectiveInputCommand {
	t.Helper()
	return PinDetectiveInputCommand{OpID: "pin", SessionID: "s1", InputSnapshotID: "input", RunID: "run", ExpectedVersion: 1, ExpectedContextSetID: v.Input.ContextSetID, Scope: v.Input.Scope, Pin: *v.Input.Detective, NativeBinding: detective.OperationBinding{Actor: p.Actor, Project: v.Input.Detective.Dispatch.NativeProjectID, Action: "run-check", ResourceID: "check", PayloadDigest: v.Input.Detective.Dispatch.NativeRequestDigest}}
}

func TestDetectivePinSaveAndReadFailuresAreAtomic(t *testing.T) {
	for _, collection := range []string{Inputs, Runs, DetectiveWork, Sessions, Operations, Audit} {
		t.Run("save-"+collection, func(t *testing.T) {
			s, p := pinService(t)
			c := pinCommand(t, s, p)
			base := s.repo
			s.repo = &interceptedRepository{Repository: base, failSave: func(actual, _ string) error {
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
	for _, collection := range []string{Sessions, Redactions, Operations, ContextSets} {
		t.Run("load-"+collection, func(t *testing.T) {
			s, p := pinService(t)
			c := pinCommand(t, s, p)
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
	for _, collection := range []string{Inputs, Runs} {
		t.Run("replay-"+collection, func(t *testing.T) {
			s, p := pinService(t)
			c := pinCommand(t, s, p)
			if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
				t.Fatal(err)
			}
			s.repo = &interceptedRepository{Repository: s.repo, failLoad: func(actual, _ string) error {
				if actual == collection {
					return ErrUnavailable
				}
				return nil
			}}
			if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrUnavailable) {
				t.Fatal(err)
			}
		})
	}
}

func TestDetectivePinAdmissionFailsBeforeMutation(t *testing.T) {
	cases := map[string]func(*Service, *Principal, *PinDetectiveInputCommand){
		"disabled": func(s *Service, _ *Principal, _ *PinDetectiveInputCommand) { s.nativeNamespace = nil },
		"executor": func(_ *Service, p *Principal, _ *PinDetectiveInputCommand) { p.Executor = false },
		"namespace": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.Pin.Reference.NamespaceID = strings.Repeat("f", 64)
		},
		"identity":   func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.Scope.IdentityDomain = "other" },
		"command-id": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.OpID = "../bad" },
		"empty-set": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.Pin.EffectiveInputs["empty"] = detective.InputValue{Set: &apicontract.TypedValueSet{}}
		},
		"invalid-value": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.Pin.EffectiveInputs["bad"] = detective.InputValue{}
		},
		"definition":   func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.Pin.Definition.SchemaVersion = 2 },
		"missing-case": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.Pin.Dispatch.CaseID = "case" },
		"invalid-effective": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.Pin.EffectiveInputs["extra"] = detective.InputValue{Scalar: valuePtr(apicontract.NewIntegerValue("1"))}
		},
		"action":         func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.Pin.Dispatch.Action = "run-query" },
		"native-binding": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.NativeBinding.Actor = "mallory" },
		"native-digest": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.NativeBinding.PayloadDigest = "bad"
			c.Pin.Dispatch.NativeRequestDigest = "bad"
		},
		"version": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.ExpectedVersion++ },
		"context-set": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) {
			c.ExpectedContextSetID = "missing"
			c.Pin.Reference.ContextSetID = "missing"
		},
		"scope": func(_ *Service, _ *Principal, c *PinDetectiveInputCommand) { c.Scope.EnvironmentID = "" },
		"denied": func(s *Service, _ *Principal, _ *PinDetectiveInputCommand) {
			s.authorize = func(context.Context, Access) error { return errors.New("private provider failure") }
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			s, p := pinService(t)
			c := pinCommand(t, s, p)
			original := c
			change(s, &p, &c)
			if _, err := s.PinDetectiveInput(context.Background(), p, c); err == nil || strings.Contains(err.Error(), "private provider") {
				t.Fatalf("unsafe admission: %v", err)
			}
			assertNoPin(t, s, p, original)
		})
	}
}

func TestDetectivePinCaseInputsAndIsolation(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	scalar := detective.InputValue{Scalar: valuePtr(apicontract.NewIntegerValue("901"))}
	set, _ := apicontract.NewTypedValueSet(apicontract.NewIntegerValue("902"), apicontract.NewIntegerValue("903"))
	values := map[string]detective.InputValue{"scalar": scalar, "set": {Set: &set}, "range": {Range: &detective.ValueRange{Type: "range", Start: apicontract.NewIntegerValue("904"), End: apicontract.NewIntegerValue("905")}}}
	for _, name := range []string{"scalar", "set", "range"} {
		c.Pin.Definition.Check.Inputs = append(c.Pin.Definition.Check.Inputs, detective.Input{Name: name, Type: "integer", Cardinality: name, Required: true})
		c.Pin.Reference.Bindings = append(c.Pin.Reference.Bindings, apicontract.SessionInputBindingRef{InputName: name, Origin: "case", CaseID: "case", CaseVersion: 2})
	}
	c.Pin.EffectiveInputs = values
	c.Case = &detective.Case{ID: "case", Version: 2, Status: "active", Context: values}
	bindPin(t, p, &c)
	bad := clonePinCommand(t, c)
	bad.Pin.EffectiveInputs["scalar"] = detective.InputValue{Scalar: valuePtr(apicontract.NewIntegerValue("999"))}
	if _, err := s.PinDetectiveInput(context.Background(), p, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Case replacement: %v", err)
	}
	bad = clonePinCommand(t, c)
	bad.Pin.Reference.Bindings[0].CaseVersion = 3
	if _, err := s.PinDetectiveInput(context.Background(), p, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Case provenance: %v", err)
	}
	bad = clonePinCommand(t, c)
	bad.Pin.Dispatch.CaseVersion = 3
	if _, err := s.PinDetectiveInput(context.Background(), p, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Case version: %v", err)
	}
	bad = clonePinCommand(t, c)
	bad.Pin.Dispatch.CaseContextDigest = strings.Repeat("0", 64)
	if _, err := s.PinDetectiveInput(context.Background(), p, bad); !errors.Is(err, ErrInvalid) {
		t.Fatalf("Case digest: %v", err)
	}
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	values["scalar"].Scalar.Str = "999"
	set.Values[0].Str = "999"
	values["range"].Range.Start.Str = "999"
	got, err := s.ReadDetectiveDispatch(context.Background(), p, "s1", "run")
	if err != nil || got.EffectiveInputs["scalar"].Scalar.Str != "901" || got.EffectiveInputs["set"].Set.Values[0].Str != "902" || got.EffectiveInputs["range"].Range.Start.Str != "904" {
		t.Fatalf("Case snapshot aliases caller: %v", err)
	}
	foreign := p
	foreign.Actor = "mallory"
	if _, err = s.ReadDetectiveDispatch(context.Background(), foreign, "s1", "run"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = s.ReadDetectiveDispatch(context.Background(), p, "foreign", "run"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	s.authorize = func(context.Context, Access) error { return ErrForbidden }
	if _, err = s.ReadDetectiveDispatch(context.Background(), p, "s1", "run"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}

func TestDetectivePinConcurrentWritersAndLifecycleGuards(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			cmd := clonePinCommand(t, c)
			cmd.OpID = id
			cmd.RunID = id
			cmd.InputSnapshotID = id
			cmd.Pin.Reference.InputSnapshotID = id
			bindPin(t, p, &cmd)
			_, err := s.PinDetectiveInput(context.Background(), p, cmd)
			errs <- err
		}(id)
	}
	wg.Wait()
	close(errs)
	wins := 0
	for err := range errs {
		if err == nil {
			wins++
		} else if !errors.Is(err, ErrConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 {
		t.Fatalf("winners=%d", wins)
	}
	for _, state := range []string{"stopped", "archived", "deleted"} {
		t.Run(state, func(t *testing.T) {
			s, p := pinService(t)
			c := pinCommand(t, s, p)
			if err := s.repo.Transact(context.Background(), func(tx Transaction) error {
				var v Session
				version, e := tx.Load(context.Background(), Sessions, "s1", &v)
				if e != nil {
					return e
				}
				v.State = state
				v.Version++
				return tx.Save(context.Background(), Sessions, v.ID, v, version, p.Actor, v.ID)
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrReadOnly) {
				t.Fatal(err)
			}
			c.ExpectedVersion++
			assertNoPin(t, s, p, c)
		})
	}
}

func TestDetectivePinRejectsGenericCompletionAndRedactedContent(t *testing.T) {
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	accepted, err := s.PinDetectiveInput(context.Background(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"completeRun", "reportClientRun", "reportDashboard"} {
		if _, err = s.Execute(context.Background(), p, Command{OpID: action, Action: action, SessionID: "s1", RunID: "run", ExpectedRunVersion: 1, RunState: "completed", Kind: "check"}); err == nil {
			t.Fatalf("generic %s settled native work", action)
		}
	}
	if _, err = s.Execute(context.Background(), p, Command{OpID: "delete", Action: "transitionSession", SessionID: "s1", ExpectedVersion: accepted.Receipt.SessionVersion, Transition: "delete"}); err == nil {
		t.Fatal("pending delete accepted")
	}
	if err = s.repo.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Redactions, "s1", SessionRedaction{SessionID: "s1", Owner: p.Actor, Namespace: *s.nativeNamespace, Version: 1}, 0, p.Actor, "s1")
	}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PinDetectiveInput(context.Background(), p, c); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, _, err = s.ReadInput(context.Background(), p, "input"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err = s.ReadDetectiveDispatch(context.Background(), p, "s1", "run"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, err = s.GetSnapshot(context.Background(), p, "s1"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	listed, err := s.ListSessions(context.Background(), p, "", 10)
	if err != nil || len(listed.Items) != 0 {
		t.Fatalf("redacted content listed: %v", err)
	}
	if _, err = s.Execute(context.Background(), p, Command{OpID: "new-entry", Action: "clearBag", SessionID: "s1", BagID: "b1", ExpectedVersion: 2}); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
}

func TestNativePinNamespaceBindsActualPrivateParent(t *testing.T) {
	mounted := testMountedDatabase{"db1", dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	parent := record.NewKeyWithID("spaces", "a")
	a, err := NewNativePinRepository(mounted, parent, "host", "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	parent.ID = "changed"
	b, err := NewNativePinRepository(mounted, record.NewKeyWithID("spaces", "b"), "host", "b", "p")
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewNativePinRepository(mounted, record.NewKeyWithID("spaces", "a"), "host", "a", "p")
	if err != nil {
		t.Fatal(err)
	}
	if a.NativeNamespace() != c.NativeNamespace() || a.NativeNamespace() == b.NativeNamespace() {
		t.Fatal("private namespaces collide or alias parent")
	}
	for _, v := range []struct {
		mount                 MountedDatabase
		parent                *record.Key
		store, space, project string
	}{{nil, nil, "local", "", "p"}, {mounted, nil, "host", "a", "p"}, {mounted, nil, "local", "a", "p"}, {mounted, nil, "../bad", "", "p"}, {mounted, record.NewKeyWithID("spaces", 123), "host", "a", "p"}, {mounted, record.NewKeyWithID(" ", "bad"), "host", "a", "p"}} {
		if _, err := NewNativePinRepository(v.mount, v.parent, v.store, v.space, v.project); err == nil {
			t.Fatal("invalid namespace accepted")
		}
	}
	deep := record.NewKeyWithID("spaces", "root")
	for range 17 {
		deep = record.NewKeyWithParentAndID(deep, "folders", "x")
	}
	if _, err = NewNativePinRepository(mounted, deep, "host", "a", "p"); err == nil {
		t.Fatal("unbounded private parent accepted")
	}
	ordinary, _ := NewDALRepository(mounted, nil)
	if _, err = NewNativePinService(ordinary, func(context.Context, Access) error { return nil }); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err = NewNativePinService(&interceptedRepository{Repository: ordinary}, func(context.Context, Access) error { return nil }); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	if _, err = NewNativePinService(a, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

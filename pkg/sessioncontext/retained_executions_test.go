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
	"github.com/datatug/datatug-core/pkg/datatug"
)

var ordinaryLocalDefinition = json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[{"id":"source-query","queryType":"SQL","text":"select 1","connectionId":"native-source"}]}`)

func TestRetainedSessionSourcesPreserveOrdinaryPinsThroughClearStopAndCompletion(t *testing.T) {
	ctx := context.Background()
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "ordinary", "bag")
	scope := Scope{"uat", "tenant-a"}
	subject := Subject{ConceptID: "external-concept", FieldID: "external-field", FieldMappingRef: "external-mapping"}
	saved := executeOK(t, svc, p, Command{Action: "putEntry", OpID: "put", SessionID: "ordinary", ExpectedVersion: created.SessionVersion, BagID: "bag", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Subject: &subject, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("9223372036854775807"))}})
	pinned := executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "ordinary", ExpectedVersion: saved.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "opaque-client-provenance", DefinitionRevision: "opaque-caller-revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &scope, Attachments: []EntryRef{{EntryID: "entry", RevisionID: "revision"}}})
	cleared := executeOK(t, svc, p, Command{Action: "clearBag", OpID: "clear", SessionID: "ordinary", ExpectedVersion: pinned.SessionVersion, BagID: "bag", ExpectedBagVersion: 2, Scope: &scope})
	stopped := executeOK(t, svc, p, Command{Action: "transitionSession", OpID: "stop", SessionID: "ordinary", ExpectedVersion: cleared.SessionVersion, Transition: "stop"})
	read := func() RetainedInventory {
		inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}
	sources, err := ProjectRetainedSessionSources(ctx, read())
	if err != nil || len(sources.Ordinary) != 1 || !sources.Ordinary[0].Active || len(sources.Native) != 0 {
		t.Fatalf("stopped accepted pin lost activity: %+v %v", sources, err)
	}
	item := sources.Ordinary[0]
	if item.DefinitionID != "opaque-client-provenance" || item.DefinitionRevision != "opaque-caller-revision" || !strings.Contains(string(item.DefinitionDocument), "source-query") || len(item.Subjects) != 1 || !reflect.DeepEqual(item.Subjects[0], RetainedScopedSubject{Scope: scope, Subject: subject}) {
		t.Fatalf("pin identity or retained cleared subject lost: %+v", item)
	}
	encoded, _ := json.Marshal(item)
	if strings.Contains(string(encoded), "9223372036854775807") {
		t.Fatal("context source values escaped reference projection")
	}
	executeOK(t, svc, p, Command{Action: "reportClientRun", OpID: "complete", SessionID: "ordinary", ExpectedVersion: stopped.SessionVersion, RunID: "run", ExpectedRunVersion: 1, Kind: "query", RunState: "completed", ResultRefs: []string{"local-result"}})
	inventory := read()
	sources, err = ProjectRetainedSessionSources(ctx, inventory)
	if err != nil || sources.Ordinary[0].Active || sources.Ordinary[0].ResultRefs[0] != "local-result" || len(sources.Ordinary[0].Subjects) != 1 {
		t.Fatalf("completed retained pin lost references: %+v %v", sources, err)
	}
	sources.Ordinary[0].ResultRefs[0] = "changed"
	sources.Ordinary[0].DefinitionDocument[0] = '!'
	again, err := ProjectRetainedSessionSources(ctx, inventory)
	if err != nil || again.Ordinary[0].ResultRefs[0] != "local-result" || again.Ordinary[0].DefinitionDocument[0] != '{' {
		t.Fatal("projection mutated immutable inventory")
	}
}

func TestRetainedSessionSourcesCombineNativeAndOrdinaryAcrossOwners(t *testing.T) {
	_, svc, p, _, _, _, read := retainedDetectiveFixture(t, true)
	bob := p
	bob.Actor = "bob"
	bob.Executor = false
	created := createTestSession(t, svc, bob, "ordinary", "ordinary-bag")
	executeOK(t, svc, bob, Command{Action: "pinInput", OpID: "ordinary-pin", SessionID: "ordinary", ExpectedVersion: created.SessionVersion, InputSnapshotID: "ordinary-input", RunID: "ordinary-run", Kind: "query", DefinitionID: "local-only-id", DefinitionRevision: "revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}})
	sources, err := ProjectRetainedSessionSources(context.Background(), read())
	if err != nil || len(sources.Native) != 1 || len(sources.Ordinary) != 1 || sources.Native[0].Owner != "alice" || sources.Ordinary[0].Owner != "bob" || !sources.Native[0].Active || !sources.Ordinary[0].Active {
		t.Fatalf("mixed owners/execution authority lost: %+v %v", sources, err)
	}
	if sources.Native[0].Dispatch.NativeProjectID == sources.Native[0].Namespace.ProjectID {
		t.Fatal("fixture must preserve distinct native target namespace")
	}
	if sources.Ordinary[0].DefinitionID != "local-only-id" {
		t.Fatal("document-local provenance was reinterpreted as saved query identity")
	}
}

func TestRetainedSessionSourcesRejectOrphanAndCorruptOrdinaryHistory(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "ordinary", "bag")
	executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "ordinary", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "opaque", DefinitionRevision: "revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}})
	inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"digest", "origin", "owner", "missing input", "unknown outcome", "false completion", "pending count", "future schema", "orphan input"} {
		t.Run(mode, func(t *testing.T) {
			collection, id := Inputs, "input"
			if mode == "missing input" || mode == "unknown outcome" || mode == "false completion" {
				collection, id = Runs, "run"
			}
			if mode == "pending count" {
				collection, id = Sessions, "ordinary"
			}
			bad := mutateRetainedContext(t, inventory, collection, id, func(d *RetainedDocument) {
				if mode == "owner" {
					d.Owner = "bob"
					return
				}
				var body map[string]any
				if err := json.Unmarshal(d.Payload, &body); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "digest":
					body["definitionDigest"] = "changed"
				case "origin":
					body["parametersOrigin"] = "server-executor"
				case "missing input":
					body["inputSnapshotId"] = "missing"
				case "unknown outcome":
					body["state"] = "future"
				case "false completion":
					body["state"] = "completed"
				case "pending count":
					body["pendingRunCount"] = 0
				case "future schema":
					body["futureReference"] = "unresolved"
				case "orphan input":
					d.ID = "orphan"
					body["id"] = "orphan"
				}
				d.Payload, _ = json.Marshal(body)
			})
			if out, err := ProjectRetainedSessionSources(context.Background(), bad); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
				t.Fatalf("unknown retained usage returned sources: %+v %v", out, err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if out, err := ProjectRetainedSessionSources(ctx, inventory); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("canceled source reader returned partial data")
	}
}

func TestRetainedSessionSourcesOwnOrdinaryOutcomeAndExecutionContracts(t *testing.T) {
	for _, kind := range []string{"query", "dashboard", "chat"} {
		for _, executor := range []bool{false, true} {
			outcomes := []string{"completed", "failed"}
			if !executor {
				outcomes = append(outcomes, "outcome-unknown")
			}
			for _, outcome := range outcomes {
				authority := "client"
				if executor {
					authority = "server"
				}
				t.Run(kind+"/"+authority+"/"+outcome, func(t *testing.T) {
					repo, db := retainedInventoryFixture(t)
					svc, err := NewService(repo, func(context.Context, Access) error { return nil })
					if err != nil {
						t.Fatal(err)
					}
					p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID, Executor: executor}
					created := createTestSession(t, svc, p, "session", "bag")
					pinned := executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "session", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: kind, DefinitionID: "opaque", DefinitionRevision: "revision", DefinitionDocument: testDefinition(kind), Scope: &Scope{"uat", "tenant-a"}})
					version, runVersion := pinned.SessionVersion, int64(1)
					if kind == "chat" {
						recorded := executeOK(t, svc, p, Command{Action: "recordRunExecution", OpID: "execution", SessionID: "session", ExpectedVersion: version, RunID: "run", ExpectedRunVersion: runVersion, ExecutionDocument: ordinaryLocalDefinition, ResolvedParameters: map[string]apicontract.TypedValue{"private": apicontract.NewStringValue("private-parameter-value")}})
						version, runVersion = recorded.SessionVersion, runVersion+1
					}
					read := func() RetainedInventory {
						v, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
						if err != nil {
							t.Fatal(err)
						}
						return v
					}
					accepted, err := ProjectRetainedSessionSources(context.Background(), read())
					if err != nil || len(accepted.Ordinary) != 1 || !accepted.Ordinary[0].Active {
						t.Fatalf("accepted %s pin: %+v %v", kind, accepted, err)
					}
					command := Command{Action: "reportClientRun", OpID: "outcome", SessionID: "session", ExpectedVersion: version, RunID: "run", ExpectedRunVersion: runVersion, Kind: kind, RunState: outcome}
					if executor {
						command.Action = "completeRun"
					}
					if outcome == "failed" {
						command.ErrorCode = "SOURCE_UNAVAILABLE"
					}
					if outcome == "outcome-unknown" {
						command.ErrorCode = "CLIENT_OUTCOME_UNKNOWN"
					}
					if outcome == "completed" {
						command.ResultRefs = []string{"result"}
						if kind == "dashboard" {
							command.Widgets = []WidgetOutcome{{ID: "w1", State: "completed", ResultRef: "widget-result"}}
						}
						if kind == "chat" && !executor {
							command.Text = "Private assistant response"
						}
					}
					executeOK(t, svc, p, command)
					inventory := read()
					sources, err := ProjectRetainedSessionSources(context.Background(), inventory)
					if err != nil || len(sources.Ordinary) != 1 || sources.Ordinary[0].Active != (outcome == "outcome-unknown") || sources.Ordinary[0].State != outcome || sources.Ordinary[0].ExecutionAuthority != authority {
						t.Fatalf("retained outcome: %+v %v", sources, err)
					}
					snapshot, err := svc.GetSnapshot(context.Background(), p, "session")
					if err != nil || snapshot.Session.PendingRunCount != 0 {
						t.Fatal("settled client bookkeeping remained operationally pending", snapshot.Session, err)
					}
					if outcome == "outcome-unknown" {
						// The owning session identity is certain even when an external
						// execution's completion is not. A provider binding this run must
						// preserve active use; opaque DefinitionID is never guessed here.
						run := sources.Ordinary[0]
						target := datatug.ProjectObjectRef{Kind: "session", ID: run.LifecycleSessionID}
						objects := []datatug.ProjectObjectUsage{{Ref: target, ContentRevision: fmt.Sprint(snapshot.Session.Version)}, {Ref: datatug.ProjectObjectRef{Kind: "execution", ID: run.RunID}, ContentRevision: run.DefinitionDigest, Historical: !run.Active, References: []datatug.ProjectObjectRef{target}}}
						usage := datatug.ProjectUsageSnapshot{Complete: true, Revision: "test", Objects: objects}
						for _, action := range []string{"archive", "delete"} {
							_, err := datatug.PlanProjectLifecycle(usage, datatug.ProjectLifecycleWrite{OperationID: "protect-unknown", ExpectedRevision: usage.Revision, Change: datatug.ProjectLifecycleChange{Object: target, Action: action}})
							if !errors.Is(err, datatug.ErrProjectObjectReferenced) {
								t.Fatalf("uncertain execution allowed %s: %v", action, err)
							}
						}
						badPending := mutateRetainedContext(t, inventory, Sessions, "session", func(document *RetainedDocument) {
							var session Session
							if err := json.Unmarshal(document.Payload, &session); err != nil {
								t.Fatal(err)
							}
							session.PendingRunCount = 1
							document.Payload, _ = json.Marshal(session)
						})
						if out, err := ProjectRetainedSessionSources(context.Background(), badPending); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
							t.Fatal("lifecycle activity was counted as operationally pending", err)
						}
					}
					if kind == "chat" && !strings.Contains(string(sources.Ordinary[0].ExecutionDocument), "native-source") {
						t.Fatal("chat executable source pin lost")
					}
					if kind == "dashboard" && outcome == "completed" && sources.Ordinary[0].Widgets[0].ResultRef != "widget-result" {
						t.Fatal("widget artifact reference lost")
					}
					encoded, _ := json.Marshal(sources.Ordinary)
					if strings.Contains(string(encoded), "private-parameter-value") || strings.Contains(string(encoded), "Private assistant response") {
						t.Fatal("private parameter values or prose escaped source descriptor")
					}
					if kind == "chat" {
						bad := mutateRetainedContext(t, inventory, Runs, "run", func(d *RetainedDocument) {
							var body map[string]any
							_ = json.Unmarshal(d.Payload, &body)
							body["executionDigest"] = "changed"
							d.Payload, _ = json.Marshal(body)
						})
						if out, err := ProjectRetainedSessionSources(context.Background(), bad); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
							t.Fatal("chat execution digest mismatch accepted")
						}
					}
				})
			}
		}
	}
}

func TestRetainedSessionSourcesShareSubjectBudgetAcrossContextAndExecution(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "session", "bag")
	version := created.SessionVersion
	refs := make([]EntryRef, 0, 400)
	for i := range 400 {
		entry, revision := fmt.Sprintf("entry-%d", i), fmt.Sprintf("revision-%d", i)
		put := executeOK(t, svc, p, Command{Action: "putEntry", OpID: fmt.Sprintf("put-%d", i), SessionID: "session", ExpectedVersion: version, BagID: "bag", ExpectedBagVersion: int64(i + 1), EntryID: entry, RevisionID: revision, Subject: &Subject{ConceptID: "external", FieldID: "field", FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}})
		version = put.SessionVersion
		refs = append(refs, EntryRef{entry, revision})
	}
	read := func() RetainedInventory {
		inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 10000, MaxBytes: 16 << 20})
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}
	for i := range 23 {
		pin := executeOK(t, svc, p, Command{Action: "pinInput", OpID: fmt.Sprintf("pin-%d", i), SessionID: "session", ExpectedVersion: version, InputSnapshotID: fmt.Sprintf("input-%d", i), RunID: fmt.Sprintf("run-%d", i), Kind: "query", DefinitionID: "opaque", DefinitionRevision: "revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}, Attachments: refs})
		version = pin.SessionVersion
		if i == 21 {
			sources, err := ProjectRetainedSessionSources(context.Background(), read())
			if err != nil {
				t.Fatalf("exact aggregate subject capacity rejected: %v", err)
			}
			count := 0
			for _, item := range sources.Contexts {
				count += len(item.Subjects)
			}
			for _, item := range sources.Ordinary {
				count += len(item.Subjects)
			}
			if count != 10000 {
				t.Fatalf("boundary fixture has %d subjects", count)
			}
		}
	}
	if out, err := ProjectRetainedSessionSources(context.Background(), read()); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("separate family allowances bypassed aggregate subject capacity")
	}
}

func TestRetainedSessionSourcesFlattenNativePinnedSubjectsAfterClear(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	svc, err := NewNativePinService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID, Executor: true}
	command := pinCommand(t, svc, p)
	subject := Subject{ConceptID: "external", FieldID: "field", FieldMappingRef: "mapping"}
	put := executeOK(t, svc, p, Command{Action: "putEntry", OpID: "put-native-context", SessionID: "s1", ExpectedVersion: command.ExpectedVersion, BagID: "b1", ExpectedBagVersion: 1, EntryID: "entry", RevisionID: "revision", Subject: &subject, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}})
	command.ExpectedVersion = put.SessionVersion
	command.Attachments = []EntryRef{{EntryID: "entry", RevisionID: "revision"}}
	bindPin(t, p, &command)
	pinned, err := svc.PinDetectiveInput(context.Background(), p, command)
	if err != nil {
		t.Fatal(err)
	}
	executeOK(t, svc, p, Command{Action: "clearBag", OpID: "clear-native-context", SessionID: "s1", ExpectedVersion: pinned.Receipt.SessionVersion, BagID: "b1", ExpectedBagVersion: 2, Scope: &command.Scope})
	inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := ProjectRetainedSessionSources(context.Background(), inventory)
	if err != nil || len(sources.Native) != 1 || !sources.Native[0].Active || len(sources.Native[0].Subjects) != 1 || !reflect.DeepEqual(sources.Native[0].Subjects[0], RetainedScopedSubject{command.Scope, subject}) {
		t.Fatalf("native active context dependency lost after clear: %+v %v", sources, err)
	}
}

func TestRetainedSessionSourcesKeepOrdinaryLegacyHandoffUnknown(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID, Executor: true}
	created := createTestSession(t, svc, p, "session", "bag")
	pin := executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "session", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "opaque", DefinitionRevision: "revision", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}})
	executeOK(t, svc, p, Command{Action: "completeRun", OpID: "complete", SessionID: "session", ExpectedVersion: pin.SessionVersion, RunID: "run", ExpectedRunVersion: 1, RunState: "completed", CaseID: "case", DetectiveRunID: "native-run"})
	inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := ProjectRetainedSessionSources(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("ordinary legacy Case handoff was silently dropped")
	}
}

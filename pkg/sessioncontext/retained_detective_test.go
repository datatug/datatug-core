package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/datatug/datatug-core/pkg/detective"
)

func retainedDetectiveFixture(t *testing.T, withCase bool) (RetainedInventory, *Service, Principal, *DetectiveOwner, DetectiveIdentity, *DetectiveObservation, func() RetainedInventory) {
	t.Helper()
	repo, db := retainedInventoryFixture(t)
	svc, err := NewNativePinService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.databaseID, ProjectID: repo.nativeNamespace.ProjectID, Executor: true}
	command := pinCommand(t, svc, p)
	// Preserve a folder-qualified saved identity all the way through the pin.
	command.Pin.Definition.Check.Steps[0].QueryID = "reference/q"
	command.Pin.Definition.Queries[0].QueryID = "reference/q"
	command.Pin.Definition.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"q","folderPath":"reference","title":"Query","type":"SQL","text":"select 1"}`)
	if withCase {
		command.Case = &detective.Case{ID: "case", Version: 1, Status: "active"}
	}
	bindPin(t, p, &command)
	if _, err = svc.PinDetectiveInput(context.Background(), p, command); err != nil {
		t.Fatal(err)
	}
	d := command.Pin.Dispatch
	id := DetectiveIdentity{repo.nativeNamespace, p.Actor, command.SessionID, command.RunID, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
	observation := &DetectiveObservation{Identity: id, FenceGeneration: 1}
	owner, err := NewDetectiveOwner(svc, func(context.Context, Principal, DetectiveIdentity) error { return nil }, func(context.Context, DetectiveIdentity) (DetectiveObservation, error) { return *observation, nil })
	if err != nil {
		t.Fatal(err)
	}
	read := func() RetainedInventory {
		t.Helper()
		inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20})
		if err != nil {
			t.Fatal(err)
		}
		return inventory
	}
	return read(), svc, p, owner, id, observation, read
}

func TestRetainedDetectiveProjectionPreservesActiveAndCompletedPinnedDependencies(t *testing.T) {
	inventory, _, p, owner, id, observation, read := retainedDetectiveFixture(t, true)
	items, err := ProjectRetainedDetectiveExecutions(context.Background(), inventory)
	if err != nil || len(items) != 1 || !items[0].Active || items[0].HandoffActive || items[0].Definition.Queries[0].QueryID != "reference/q" || items[0].Dispatch.CaseID != "case" || items[0].Namespace != inventory.Namespace || items[0].Dispatch.NativeProjectID == inventory.Namespace.ProjectID {
		t.Fatalf("invalid active projection: %+v %v", items, err)
	}
	// Caller-owned output cannot mutate the immutable inventory or persisted pin.
	items[0].Definition.Queries[0].DefinitionJSON[0] = 'x'
	if _, err = ProjectRetainedDetectiveExecutions(context.Background(), inventory); err != nil {
		t.Fatal(err)
	}
	if _, _, err = owner.ClaimDispatch(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	completeObservation(observation)
	if _, err = owner.Reconcile(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	items, err = ProjectRetainedDetectiveExecutions(context.Background(), read())
	if err != nil || len(items) != 1 || items[0].Active || !items[0].HandoffActive || items[0].Definition.Queries[0].QueryID != "reference/q" {
		t.Fatalf("completed dependencies lost: %+v %v", items, err)
	}
	sources, err := ProjectRetainedSessionSources(context.Background(), read())
	if err != nil || len(sources.Native) != 1 || sources.Native[0].Active || !sources.Native[0].HandoffActive {
		t.Fatalf("native settlement metadata or pending handoff omitted: %+v %v", sources, err)
	}
	foundSettlement := false
	for _, item := range sources.Provenance {
		if item.Collection == Audit && item.Action == "nativeComplete" {
			foundSettlement = true
		}
	}
	if !foundSettlement {
		t.Fatal("native audit-only settlement omitted")
	}
	_, version := ownerHandoff(t, owner.service, id)
	if err = owner.SettleHandoff(context.Background(), p, id, version, true, "native-receipt"); err != nil {
		t.Fatal(err)
	}
	items, err = ProjectRetainedDetectiveExecutions(context.Background(), read())
	if err != nil || items[0].Active || items[0].HandoffActive || len(items[0].Definition.Queries) != 1 {
		t.Fatalf("delivered history lost dependencies: %+v %v", items, err)
	}
}

func TestRetainedDetectiveProjectionDoesNotTreatAFenceAsCompletion(t *testing.T) {
	_, _, p, owner, id, _, read := retainedDetectiveFixture(t, false)
	if err := owner.Fence(context.Background(), p, id, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	items, err := ProjectRetainedDetectiveExecutions(context.Background(), read())
	if err != nil || len(items) != 1 || !items[0].Active {
		t.Fatalf("fence incorrectly made retained acceptance inactive: %+v %v", items, err)
	}
}

func TestRetainedDetectiveProjectionRejectsUnknownOrBrokenRetainedHistoryWithoutPartialOutput(t *testing.T) {
	for _, mode := range []string{"unsealed", "redacted pin", "orphan input", "orphan owner", "wrong namespace", "foreign input", "wrong actor", "digest", "ordinary pin", "unknown schema", "false completion", "manifest", "duplicate", "mixed document", "mixed digest"} {
		t.Run(mode, func(t *testing.T) {
			inventory, _, _, _, _, _, _ := retainedDetectiveFixture(t, false)
			mutate := func(collection string, change func(map[string]any)) {
				for i := range inventory.Documents {
					if inventory.Documents[i].Collection == collection {
						var raw map[string]any
						if err := json.Unmarshal(inventory.Documents[i].Payload, &raw); err != nil {
							t.Fatal(err)
						}
						change(raw)
						inventory.Documents[i].Payload, _ = json.Marshal(raw)
						return
					}
				}
				t.Fatalf("missing %s", collection)
			}
			switch mode {
			case "unsealed":
				mutate(DetectiveWork, func(p map[string]any) { p["ownerProtocolVersion"] = 0 })
			case "redacted pin":
				mutate(Inputs, func(p map[string]any) { delete(p, "detective") })
			case "orphan input":
				mutate(Runs, func(p map[string]any) { p["inputSnapshotId"] = "missing" })
			case "orphan owner":
				for i := range inventory.Documents {
					if inventory.Documents[i].Collection == DetectiveWork {
						inventory.Documents[i].ID = "orphan"
					}
				}
			case "wrong namespace":
				inventory.Namespace.NamespaceID = "foreign"
			case "foreign input":
				mutate(Inputs, func(p map[string]any) { p["sessionId"] = "foreign" })
			case "wrong actor":
				for i := range inventory.Documents {
					if inventory.Documents[i].Collection == Operations {
						inventory.Documents[i].Owner = "bob"
					}
				}
			case "digest":
				mutate(Inputs, func(p map[string]any) { p["definitionDigest"] = "wrong" })
			case "ordinary pin":
				mutate(Runs, func(p map[string]any) { p["executionAuthority"] = "client" })
			case "unknown schema":
				mutate(Inputs, func(p map[string]any) { p["futureDependencies"] = []string{"missing-query"} })
			case "false completion":
				mutate(Runs, func(p map[string]any) { p["state"] = "completed"; p["completedAt"] = "2026-10-28T10:00:00Z" })
			case "manifest":
				inventory.Collections = inventory.Collections[1:]
			case "duplicate":
				inventory.Documents = append(inventory.Documents, inventory.Documents[0])
			case "mixed document":
				mutate(Runs, func(p map[string]any) { p["executionDocument"] = map[string]any{"connectionId": "another-source"} })
			case "mixed digest":
				mutate(Runs, func(p map[string]any) { p["executionDigest"] = "another-execution" })
			}
			items, err := ProjectRetainedDetectiveExecutions(context.Background(), inventory)
			if !errors.Is(err, ErrUnavailable) || len(items) != 0 {
				t.Fatalf("unknown %s produced partial projection: %+v %v", mode, items, err)
			}
		})
	}
}

func TestRetainedDetectiveProjectionRequiresCompleteSettledHandoffJoins(t *testing.T) {
	for _, mode := range []string{"missing", "wrong native run", "unsealed delivery", "false redaction"} {
		t.Run(mode, func(t *testing.T) {
			_, _, p, owner, id, observation, read := retainedDetectiveFixture(t, true)
			if _, _, err := owner.ClaimDispatch(context.Background(), p, id); err != nil {
				t.Fatal(err)
			}
			completeObservation(observation)
			if _, err := owner.Reconcile(context.Background(), p, id); err != nil {
				t.Fatal(err)
			}
			inventory := read()
			for i := range inventory.Documents {
				if inventory.Documents[i].Collection != Outbox {
					continue
				}
				if mode == "missing" {
					inventory.Documents = append(inventory.Documents[:i], inventory.Documents[i+1:]...)
					break
				}
				var handoff EvidenceHandoff
				if err := json.Unmarshal(inventory.Documents[i].Payload, &handoff); err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "wrong native run":
					handoff.DetectiveRunID = "foreign"
				case "unsealed delivery":
					handoff.State = "delivered"
				case "false redaction":
					handoff.State = "redacted"
				}
				inventory.Documents[i].Payload, _ = json.Marshal(handoff)
			}
			items, err := ProjectRetainedDetectiveExecutions(context.Background(), inventory)
			if !errors.Is(err, ErrUnavailable) || len(items) != 0 {
				t.Fatalf("unverified %s handoff made inactive: %+v %v", mode, items, err)
			}
		})
	}
}

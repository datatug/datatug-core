package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func retainedHistoryFixture(t *testing.T) RetainedInventory {
	t.Helper()
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "ordinary", "bag")
	pin := executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "ordinary", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "chat", DefinitionID: "opaque", DefinitionRevision: "v1", DefinitionDocument: testDefinition("chat"), Scope: &Scope{"uat", "tenant-a"}, Text: "Private prompt"})
	executeOK(t, svc, p, Command{Action: "reportClientRun", OpID: "outcome", SessionID: "ordinary", ExpectedVersion: pin.SessionVersion, RunID: "run", ExpectedRunVersion: 1, Kind: "chat", RunState: "completed", Text: "Private response"})
	executeOK(t, svc, p, Command{Action: "importLegacy", OpID: "import", LegacyImport: &LegacyImport{SourceID: "legacy", Scope: Scope{"uat", "tenant-a"}, Entries: []LegacyEntry{{ID: "legacy-entry", Subject: Subject{ConceptID: "external", FieldID: "field", FieldMappingRef: "mapping"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("1"))}}}, Messages: []LegacyChatMessage{{ID: "legacy-message", Role: "user", Text: "Private historical prose"}}}})
	inv, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProjectRetainedSessionSources(context.Background(), inv); err != nil {
		t.Fatal("invalid real history fixture", err)
	}
	return inv
}

func TestRetainedProvenanceRejectsIndependentMetadataAndResourceCorruption(t *testing.T) {
	inv := retainedHistoryFixture(t)
	original, _ := retainedReader(inv)
	var legacyID, legacySession, legacyBag, messageID string
	for _, d := range inv.Documents {
		if d.Collection == LegacyImports {
			var m LegacyImportManifest
			_ = json.Unmarshal(d.Payload, &m)
			legacyID, legacySession, legacyBag = m.ID, m.SessionID, m.BagID
		}
		if d.Collection == Messages && d.SessionID == "ordinary" {
			messageID = d.ID
		}
	}
	receiptID := opKey(Principal{Actor: "alice", DatabaseID: inv.Namespace.DatabaseID, ProjectID: inv.Namespace.ProjectID}, "pin")
	for _, mode := range []string{"recovery seal", "receipt version", "future action", "transition version", "zero resource version", "resource version", "audit orphan", "copy self", "copy missing", "message session", "message role", "message provenance", "message input", "message run", "message input owner", "duplicate run", "legacy invalid", "legacy identity", "duplicate legacy", "legacy work", "legacy bag scope", "legacy bag owner"} {
		t.Run(mode, func(t *testing.T) {
			r := retainedInventoryReader{}
			for k, v := range original {
				r[k] = v
			}
			patch := func(key string, fn func(map[string]any)) {
				t.Helper()
				d, ok := r[key]
				if !ok {
					t.Fatal("missing", key)
				}
				var b map[string]any
				if err := json.Unmarshal(d.Payload, &b); err != nil {
					t.Fatal(err)
				}
				fn(b)
				d.Payload, _ = json.Marshal(b)
				r[key] = d
			}
			changeAction := func(action string) {
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["action"] = action })
				patch(Audit+"/"+receiptID, func(b map[string]any) { b["action"] = action })
			}
			switch mode {
			case "recovery seal":
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["recoveryBindingDigest"] = strings.Repeat("a", 64) })
			case "receipt version":
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["sessionVersion"] = 999 })
			case "future action":
				changeAction("future")
			case "transition version":
				changeAction("transitionSession")
			case "zero resource version":
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["resourceVersion"] = 0 })
			case "resource version":
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["resourceVersion"] = 999 })
			case "audit orphan":
				delete(r, Operations+"/"+receiptID)
			case "copy self", "copy missing":
				changeAction("copyContext")
				patch(Operations+"/"+receiptID, func(b map[string]any) { b["resourceId"] = "ordinary" })
				patch(Audit+"/"+receiptID, func(b map[string]any) {
					b["resourceId"] = "ordinary"
					if mode == "copy missing" {
						b["resourceId"] = "missing"
					}
				})
			case "message session":
				d := r[Messages+"/"+messageID]
				d.SessionID = "missing"
				r[Messages+"/"+messageID] = d
			case "message role":
				patch(Messages+"/"+messageID, func(b map[string]any) { b["role"] = "system" })
			case "message provenance":
				patch(Messages+"/"+messageID, func(b map[string]any) { b["provenance"] = "future" })
			case "message input":
				patch(Messages+"/"+messageID, func(b map[string]any) { b["inputSnapshotId"] = "missing" })
			case "message run":
				patch(Runs+"/run", func(b map[string]any) { b["kind"] = "query" })
			case "message input owner":
				d := r[Inputs+"/input"]
				d.Owner = "bob"
				r[Inputs+"/input"] = d
			case "duplicate run":
				r[Runs+"/duplicate"] = r[Runs+"/run"]
			case "legacy invalid":
				patch(LegacyImports+"/"+legacyID, func(b map[string]any) { b["sourceDigest"] = "invalid" })
			case "legacy identity":
				patch(LegacyImports+"/"+legacyID, func(b map[string]any) { b["sourceId"] = "different" })
			case "duplicate legacy":
			case "legacy work":
				patch(Sessions+"/"+legacySession, func(b map[string]any) { b["workItemId"] = "work" })
			case "legacy bag scope":
				patch(Bags+"/"+legacyBag, func(b map[string]any) { b["scope"] = Scope{"other", "tenant-a"} })
			case "legacy bag owner":
				d := r[Bags+"/"+legacyBag]
				d.Owner = "bob"
				r[Bags+"/"+legacyBag] = d
			}
			keys := retainedKeys(r)
			if mode == "duplicate legacy" {
				keys = append([]string{LegacyImports + "/" + legacyID}, keys...)
			}
			out, err := retainedSessionProvenance(context.Background(), r, inv.Namespace, keys, &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000})
			if !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatal("corrupt provenance exposed partial reference metadata", mode, err)
			}
		})
	}
	// Cancellation in either provenance pass discards every earlier descriptor.
	for n := 1; n < 500; n++ {
		ctx := checkpointContext(n)
		out, err := retainedSessionProvenance(ctx, original, inv.Namespace, retainedKeys(original), &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000})
		canceled := ctx.Context.Err() != nil
		ctx.cancel()
		if canceled && (!errors.Is(err, context.Canceled) || out != nil) {
			t.Fatal("canceled provenance escaped", err)
		}
		if !canceled {
			break
		}
	}
}

func TestRetainedNativeSettlementAuditRejectsForeignResourceOwner(t *testing.T) {
	_, _, p, owner, id, observation, read := retainedDetectiveFixture(t, false)
	if _, _, err := owner.ClaimDispatch(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	completeObservation(observation)
	if _, err := owner.Reconcile(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	inv := read()
	r, _ := retainedReader(inv)
	d := r[Runs+"/"+id.RunID]
	d.Owner = "bob"
	r[Runs+"/"+id.RunID] = d
	if out, err := retainedSessionProvenance(context.Background(), r, inv.Namespace, retainedKeys(r), &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000}); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("foreign settlement resource admitted")
	}
}

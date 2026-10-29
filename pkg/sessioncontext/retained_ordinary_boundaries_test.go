package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func retainedOrdinaryFixture(t *testing.T) RetainedInventory {
	t.Helper()
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	created := createTestSession(t, svc, p, "ordinary", "bag")
	executeOK(t, svc, p, Command{Action: "pinInput", OpID: "pin", SessionID: "ordinary", ExpectedVersion: created.SessionVersion, InputSnapshotID: "input", RunID: "run", Kind: "query", DefinitionID: "opaque", DefinitionRevision: "v1", DefinitionDocument: ordinaryLocalDefinition, Scope: &Scope{"uat", "tenant-a"}})
	inv, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

func TestRetainedOrdinaryExecutionRejectsMixedAndUnprovenOutcomes(t *testing.T) {
	accepted := retainedOrdinaryFixture(t)
	terminal := retainedHistoryFixture(t)
	for _, mode := range []string{"missing applied context", "orphan input", "unknown kind", "mixed execution", "unbound parameters", "accepted results", "unknown outcome origin", "invalid outcome", "terminal origin"} {
		t.Run(mode, func(t *testing.T) {
			inv := accepted
			collection, id := Runs, "run"
			if mode == "unknown outcome origin" || mode == "invalid outcome" || mode == "terminal origin" {
				inv = terminal
			}
			if mode == "missing applied context" || mode == "orphan input" {
				collection, id = Inputs, "input"
			}
			bad := mutateRetainedBody(t, inv, collection, id, func(b map[string]any) {
				switch mode {
				case "missing applied context":
					b["contextSetId"] = "missing"
				case "orphan input":
					b["id"] = "orphan"
				case "unknown kind":
					b["kind"] = "future"
				case "mixed execution":
					b["executionDocument"] = json.RawMessage(ordinaryLocalDefinition)
					b["executionDigest"] = "digest"
				case "unbound parameters":
					b["resolvedParameters"] = map[string]apicontract.TypedValue{"private": apicontract.NewStringValue("secret")}
				case "accepted results":
					b["resultRefs"] = []string{"unproven"}
				case "unknown outcome origin":
					b["state"] = "outcome-unknown"
				case "invalid outcome":
					b["state"] = "future"
				case "terminal origin":
					b["outcomeOrigin"] = "foreign"
				}
			})
			if mode == "orphan input" {
				for i := range bad.Documents {
					if bad.Documents[i].Collection == Inputs && bad.Documents[i].ID == "input" {
						bad.Documents[i].ID = "orphan"
						copy := bad.Documents[i]
						copy.ID = "input"
						var body map[string]any
						_ = json.Unmarshal(copy.Payload, &body)
						body["id"] = "input"
						copy.Payload, _ = json.Marshal(body)
						bad.Documents = append(bad.Documents, copy)
						break
					}
				}
			}
			if out, err := ProjectRetainedSessionSources(context.Background(), bad); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
				t.Fatal("unproven ordinary state yielded sources", mode, err)
			}
		})
	}
}

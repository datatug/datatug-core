package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func TestRetainedSessionSourcesJoinLegacyHistoryWithoutProseOrValues(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	svc, err := NewService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: repo.NativeNamespace().DatabaseID, ProjectID: repo.NativeNamespace().ProjectID}
	old := time.Date(2001, 1, 2, 3, 4, 5, 0, time.UTC)
	legacy := &LegacyImport{SourceID: "old-context", Title: "Private imported title", Scope: Scope{"uat", "tenant-a"}, Entries: []LegacyEntry{{ID: "old-entry", Subject: Subject{ConceptID: "external", FieldID: "field", FieldMappingRef: "old-mapping"}, Value: ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("9007199254740997"))}, CreatedAt: old}}, Messages: []LegacyChatMessage{{ID: "old-message", Role: "user", Text: "Private imported prose", CreatedAt: old}, {ID: "zero-time", Role: "assistant", Text: "Private timeless prose"}}}
	command := Command{Action: "importLegacy", OpID: "import", LegacyImport: legacy}
	imported := executeOK(t, svc, p, command)
	if replay := executeOK(t, svc, p, command); !reflect.DeepEqual(replay, imported) {
		t.Fatal("legacy import replay changed acknowledgment")
	}
	executeOK(t, svc, p, Command{Action: "renameSession", OpID: "rename-import", SessionID: imported.SessionID, ExpectedVersion: imported.SessionVersion, Title: "Renamed imported title"})
	inventory, err := readRetainedInventory(t, repo, db, RetainedInventoryLimits{MaxDocuments: 1000, MaxBytes: 16 << 20})
	if err != nil {
		t.Fatal(err)
	}
	sources, err := ProjectRetainedSessionSources(context.Background(), inventory)
	if err != nil {
		t.Fatal(err)
	}
	imports, messages := 0, 0
	for _, item := range sources.Provenance {
		if item.Collection == LegacyImports {
			imports++
			if item.LegacySourceID != "old-context" || item.Scopes[0] != legacy.Scope {
				t.Fatal("legacy manifest scope lost")
			}
		}
		if item.Collection == Messages {
			messages++
			if item.LegacySourceID != "old-context" || len(item.Resources) != 1 || item.Resources[0].Collection != LegacyImports {
				t.Fatal("legacy message not joined to its manifest")
			}
		}
	}
	if imports != 1 || messages != 2 {
		t.Fatalf("legacy history omitted: imports=%d messages=%d", imports, messages)
	}
	encoded, _ := json.Marshal(sources.Provenance)
	for _, private := range []string{"Private imported", "Private timeless", "9007199254740997", "Renamed imported"} {
		if strings.Contains(string(encoded), private) {
			t.Fatal("prose/value entered reference metadata")
		}
	}
	for _, collection := range []string{LegacyImports, Messages, Revisions} {
		id := ""
		for _, d := range inventory.Documents {
			if d.Collection == collection {
				id = d.ID
				break
			}
		}
		bad := mutateRetainedContext(t, inventory, collection, id, func(d *RetainedDocument) {
			var body map[string]any
			_ = json.Unmarshal(d.Payload, &body)
			body["sourceDigest"] = strings.Repeat("a", 64)
			d.Payload, _ = json.Marshal(body)
		})
		if out, err := ProjectRetainedSessionSources(context.Background(), bad); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
			t.Fatalf("broken %s legacy join accepted", collection)
		}
	}
}

func TestRetainedSessionSourcesMetadataCorruptionAndRedactionStayUnknown(t *testing.T) {
	inventory, _ := retainedContextFixture(t)
	for _, mode := range []string{"receipt actor", "receipt action", "receipt resource", "receipt future schema", "audit actor", "audit time", "audit without receipt"} {
		t.Run(mode, func(t *testing.T) {
			collection := Operations
			if strings.HasPrefix(mode, "audit") {
				collection = Audit
			}
			id := ""
			for _, d := range inventory.Documents {
				if d.Collection == collection {
					id = d.ID
					break
				}
			}
			bad := mutateRetainedContext(t, inventory, collection, id, func(d *RetainedDocument) {
				var body map[string]any
				_ = json.Unmarshal(d.Payload, &body)
				switch mode {
				case "receipt actor", "audit actor":
					body["actor"] = "different-owner"
				case "receipt action":
					body["action"] = "future-mutation"
				case "receipt resource":
					body["resourceId"] = "missing"
				case "receipt future schema":
					body["futureReference"] = "unknown"
				case "audit time":
					body["at"] = "2000-01-01T00:00:00Z"
				case "audit without receipt":
					d.ID = strings.Repeat("b", 64)
				}
				d.Payload, _ = json.Marshal(body)
			})
			if out, err := ProjectRetainedSessionSources(context.Background(), bad); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
				t.Fatalf("corrupt metadata returned references: %+v %v", out, err)
			}
		})
	}
	// Even malformed redaction markers cannot be treated as missing records or
	// used to certify an empty dependency set after executable pins were erased.
	inventory.Documents = append(inventory.Documents, RetainedDocument{Collection: Redactions, ID: "session", SessionID: "session", Owner: "alice", Version: 1, Payload: json.RawMessage(`{"future":"redaction-marker"}`)})
	if out, err := ProjectRetainedSessionSources(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("redaction loss certified empty dependencies")
	}
}

func TestRetainedSessionSourcesNativeSettlementAuditRequiresTerminalJoin(t *testing.T) {
	inventory, _, p, owner, id, observation, read := retainedDetectiveFixture(t, false)
	key := opKey(p, "nativeComplete-"+id.RunID)
	event := AuditEvent{OpID: id.NativeOperationID, Actor: p.Actor, Action: "nativeComplete", SessionID: id.SessionID, ResourceID: id.RunID, At: time.Now().UTC()}
	body, _ := json.Marshal(event)
	inventory.Documents = append(inventory.Documents, RetainedDocument{Collection: Audit, ID: key, SessionID: id.SessionID, Owner: p.Actor, Version: 1, Payload: body})
	if out, err := ProjectRetainedSessionSources(context.Background(), inventory); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("unsettled native run accepted a completion audit")
	}
	if _, _, err := owner.ClaimDispatch(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	completeObservation(observation)
	if _, err := owner.Reconcile(context.Background(), p, id); err != nil {
		t.Fatal(err)
	}
	settled := read()
	if _, err := ProjectRetainedSessionSources(context.Background(), settled); err != nil {
		t.Fatalf("actual native settlement rejected: %v", err)
	}
	badTime := mutateRetainedContext(t, settled, Audit, key, func(d *RetainedDocument) {
		var event AuditEvent
		_ = json.Unmarshal(d.Payload, &event)
		event.At = event.At.Add(-time.Hour)
		d.Payload, _ = json.Marshal(event)
	})
	if out, err := ProjectRetainedSessionSources(context.Background(), badTime); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("settlement audit time did not match its terminal run")
	}
}

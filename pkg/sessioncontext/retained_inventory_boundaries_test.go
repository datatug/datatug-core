package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type retainedScanResultTx struct {
	dal.ReadTransaction
	query func() (dal.RecordsReader, error)
}

func (tx retainedScanResultTx) ExecuteQueryToRecordsReader(context.Context, dal.Query) (dal.RecordsReader, error) {
	return tx.query()
}

func TestRetainedInventoryRejectsAdapterEnvelopeAndQueryFailures(t *testing.T) {
	repo, _ := retainedInventoryFixture(t)
	for _, mode := range []string{"query", "data type", "envelope cost"} {
		t.Run(mode, func(t *testing.T) {
			tx := retainedScanResultTx{query: func() (dal.RecordsReader, error) {
				if mode == "query" {
					return nil, errors.New("query unavailable")
				}
				var data any = "invalid envelope"
				if mode == "envelope cost" {
					data = &map[string]any{"recordId": "session", "sessionId": "session", "owner": "alice", "version": int64(1), "payload": "{}"}
				}
				row := record.NewRecordWithData(record.NewKeyWithParentAndID(repo.parent, Sessions, "session"), data)
				return &sliceRecordsReader{records: []record.Record{row}}, nil
			}}
			limits := RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20}
			if mode == "envelope cost" {
				limits.MaxBytes = 3
			}
			if out, err := repo.RetainedInventoryInTransaction(context.Background(), tx, limits); err == nil || !reflect.DeepEqual(out, RetainedInventory{}) {
				t.Fatal("adapter corruption yielded partial inventory", mode, err)
			}
		})
	}
	// Empty collections still cross cancellation checkpoints between queries and
	// before returning the final inventory. No query may hide cancellation.
	for n := 1; n < 200; n++ {
		ctx := checkpointContext(n)
		tx := retainedScanResultTx{query: func() (dal.RecordsReader, error) { return &sliceRecordsReader{}, nil }}
		out, err := repo.RetainedInventoryInTransaction(ctx, tx, RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20})
		canceled := ctx.Context.Err() != nil
		ctx.cancel()
		if canceled && (!errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, RetainedInventory{})) {
			t.Fatal("canceled scan exposed inventory", err)
		}
		if !canceled {
			break
		}
	}
}

func TestRetainedContextRejectsOrphanRowsAndCancellationDuringDecodeFailure(t *testing.T) {
	inv, _ := retainedContextFixture(t)
	for _, collection := range []string{Bags, Entries, ContextSets} {
		t.Run(collection, func(t *testing.T) {
			bad := inv
			bad.Documents = append([]RetainedDocument(nil), inv.Documents...)
			for _, d := range inv.Documents {
				if d.Collection == collection {
					d.ID = "orphan"
					var body map[string]any
					_ = json.Unmarshal(d.Payload, &body)
					body["id"] = "orphan"
					switch collection {
					case Entries:
						body["bagId"] = "missing"
					case ContextSets:
						d.SessionID = "missing"
						body["sessionId"] = "missing"
					}
					d.Payload, _ = json.Marshal(body)
					bad.Documents = append(bad.Documents, d)
					break
				}
			}
			if out, err := ProjectRetainedContextBindings(context.Background(), bad); !errors.Is(err, ErrUnavailable) || out != nil {
				t.Fatal("orphan context row accepted", collection, err)
			}
		})
	}
	old := decodeRetainedSource
	t.Cleanup(func() { decodeRetainedSource = old })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	decodeRetainedSource = func([]byte, any, ...string) error { cancel(); return errors.New("codec canceled") }
	if out, err := ProjectRetainedContextBindings(ctx, inv); !errors.Is(err, context.Canceled) || out != nil {
		t.Fatal("decode failure hid cancellation", err)
	}
}

func TestRetainedSourceCompositionRejectsFutureFamiliesAndInconsistentNativeProjection(t *testing.T) {
	inv, _, _, _, _, _, _ := retainedDetectiveFixture(t, false)
	oldReader, oldNative := readRetainedSourceInventory, projectRetainedNativeSource
	t.Cleanup(func() { readRetainedSourceInventory, projectRetainedNativeSource = oldReader, oldNative })
	future := inv
	future.Collections = append(append([]string(nil), inv.Collections...), "future-family")
	readRetainedSourceInventory = func(RetainedInventory) (retainedInventoryReader, error) { return retainedInventoryReader{}, nil }
	if out, err := ProjectRetainedSessionSources(context.Background(), future); !errors.Is(err, ErrUnavailable) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("unprojected future family certified as empty")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	readRetainedSourceInventory = func(RetainedInventory) (retainedInventoryReader, error) {
		cancel()
		return retainedInventoryReader{}, nil
	}
	if out, err := ProjectRetainedSessionSources(ctx, future); !errors.Is(err, context.Canceled) || !reflect.DeepEqual(out, RetainedSessionSources{}) {
		t.Fatal("future source cancellation hidden", err)
	}
	readRetainedSourceInventory = oldReader
	projectRetainedNativeSource = func(context.Context, NativeNamespace, retainedInventoryReader, string) (RetainedDetectiveExecution, error) {
		return RetainedDetectiveExecution{InputSnapshotID: "missing"}, nil
	}
	if out, err := ProjectRetainedDetectiveExecutions(context.Background(), inv); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("unmatched native projector output accepted")
	}
}

func TestRetainedProvenanceRejectsUnsealedNativeAudit(t *testing.T) {
	inv := retainedOrdinaryFixture(t)
	r, _ := retainedReader(inv)
	event := AuditEvent{Actor: "alice", SessionID: "ordinary", Action: "nativeComplete", ResourceID: "missing", OpID: "native", At: time.Now().UTC()}
	body, _ := json.Marshal(event)
	r[Audit+"/orphan"] = RetainedDocument{Collection: Audit, ID: "orphan", SessionID: "ordinary", Owner: "alice", Version: 1, Payload: body}
	if out, err := retainedSessionProvenance(context.Background(), r, inv.Namespace, retainedKeys(r), &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000}); !errors.Is(err, ErrUnavailable) || out != nil {
		t.Fatal("audit invented native ownership")
	}
}

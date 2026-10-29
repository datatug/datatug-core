package sessioncontext

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func retainedInventoryFixture(t *testing.T) (*DALRepository, dal.DB) {
	t.Helper()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	repo, err := NewNativePinRepository(testMountedDatabase{id: "session-db", db: db}, record.NewKeyWithParentAndID(nil, "sessionProjects", "project-a"), "firestore", "space-a", "project-a")
	if err != nil {
		t.Fatal(err)
	}
	return repo, db
}
func seedRetainedEnvelope(t *testing.T, repo *DALRepository, collection, id, owner string) {
	t.Helper()
	err := repo.Transact(context.Background(), func(tx Transaction) error {
		// These are envelope fixtures. Source adapters must additionally decode
		// each collection's actual schema before claiming reference completeness.
		return tx.Save(context.Background(), collection, id, map[string]any{"id": id, "expiresAt": "2000-01-01T00:00:00Z", "state": "completed"}, 0, owner, "session-a")
	})
	if err != nil {
		t.Fatal(err)
	}
}
func readRetainedInventory(t *testing.T, repo *DALRepository, db dal.DB, limits RetainedInventoryLimits) (RetainedInventory, error) {
	t.Helper()
	var result RetainedInventory
	err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		var err error
		result, err = repo.RetainedInventoryInTransaction(ctx, tx, limits)
		return err
	})
	return result, err
}

func TestRetainedInventoryScansEveryOwnerAndCollectionWithoutExpiryFiltering(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	for _, collection := range Collections() {
		seedRetainedEnvelope(t, repo, collection, "alice-record", "alice")
	}
	seedRetainedEnvelope(t, repo, Inputs, "bob-record", "bob")
	other, err := NewNativePinRepository(testMountedDatabase{id: "session-db", db: db}, record.NewKeyWithParentAndID(nil, "sessionProjects", "project-b"), "firestore", "space-a", "project-b")
	if err != nil {
		t.Fatal(err)
	}
	seedRetainedEnvelope(t, other, Inputs, "foreign-record", "bob")
	limits := RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20}
	got, err := readRetainedInventory(t, repo, db, limits)
	if err != nil {
		t.Fatal(err)
	}
	if got.Namespace != repo.NativeNamespace() || !reflect.DeepEqual(got.Collections, Collections()) || len(got.Documents) != len(Collections())+1 {
		t.Fatalf("incomplete project scan: %+v", got)
	}
	seen := map[string]bool{}
	for _, doc := range got.Documents {
		if doc.ID == "foreign-record" || doc.Version != 1 || doc.SessionID != "session-a" {
			t.Fatalf("scope/envelope mismatch: %+v", doc)
		}
		seen[doc.Collection+"/"+doc.Owner] = true
	}
	if !seen[Inputs+"/bob"] || !seen[Runs+"/alice"] || !seen[Redactions+"/alice"] || !seen[DetectiveOwners+"/alice"] {
		t.Fatalf("retained sources omitted: %v", seen)
	}
	got.Documents[0].Payload[0] = 'x'
	again, err := readRetainedInventory(t, repo, db, limits)
	if err != nil || again.Documents[0].Payload[0] != '{' {
		t.Fatalf("inventory mutated storage payload: %v", err)
	}
}

func TestRetainedInventoryRejectsPartialResultsAndUnboundRepositories(t *testing.T) {
	for _, scenario := range []string{"same collection overflow", "later collection overflow", "byte overflow", "unbound", "bad envelope"} {
		t.Run(scenario, func(t *testing.T) {
			repo, db := retainedInventoryFixture(t)
			seedRetainedEnvelope(t, repo, Sessions, "first", "alice")
			limits := RetainedInventoryLimits{MaxDocuments: 1, MaxBytes: 1 << 20}
			switch scenario {
			case "same collection overflow":
				seedRetainedEnvelope(t, repo, Sessions, "second", "bob")
			case "later collection overflow":
				seedRetainedEnvelope(t, repo, Inputs, "second", "bob")
			case "byte overflow":
				limits.MaxBytes = 1
			case "unbound":
				var err error
				repo, err = NewDALRepository(testMountedDatabase{id: "session-db", db: db}, repo.parent)
				if err != nil {
					t.Fatal(err)
				}
			case "bad envelope":
				err := db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(repo.parent, Sessions, "first"), map[string]any{"recordId": "different", "owner": "alice", "sessionId": "session-a", "version": 1, "payload": "{}"}))
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := readRetainedInventory(t, repo, db, limits)
			if err == nil || !reflect.DeepEqual(got, RetainedInventory{}) {
				t.Fatalf("partial/unbound inventory accepted: %+v %v", got, err)
			}
		})
	}
}

type retainedInventoryFaultTx struct {
	dal.ReadTransaction
	closeErr error
	nextErr  error
}

type cancelInventoryReader struct {
	dal.RecordsReader
	cancel context.CancelFunc
}

func (r cancelInventoryReader) Next() (record.Record, error) {
	row, err := r.RecordsReader.Next()
	r.cancel()
	return row, err
}

type cancelInventoryTx struct {
	dal.ReadTransaction
	cancel context.CancelFunc
}

func (tx cancelInventoryTx) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	reader, err := tx.ReadTransaction.ExecuteQueryToRecordsReader(ctx, q)
	if err != nil {
		return nil, err
	}
	return cancelInventoryReader{reader, tx.cancel}, nil
}
func TestRetainedInventoryHonorsCancellationEvenWhenAdapterIgnoresContext(t *testing.T) {
	for _, duringRead := range []bool{false, true} {
		repo, db := retainedInventoryFixture(t)
		seedRetainedEnvelope(t, repo, Sessions, "session-record", "alice")
		ctx, cancel := context.WithCancel(context.Background())
		if !duringRead {
			cancel()
		}
		err := db.RunReadonlyTransaction(context.Background(), func(_ context.Context, tx dal.ReadTransaction) error {
			if duringRead {
				tx = cancelInventoryTx{tx, cancel}
			}
			got, err := repo.RetainedInventoryInTransaction(ctx, tx, RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20})
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(got, RetainedInventory{}) {
				t.Fatalf("canceled scan succeeded: %+v %v", got, err)
			}
			return nil
		})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
	}
}

func (tx retainedInventoryFaultTx) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	return (closeErrorReadSession{ReadSession: tx.ReadTransaction, closeErr: tx.closeErr, nextErr: tx.nextErr}).ExecuteQueryToRecordsReader(ctx, q)
}
func TestRetainedInventoryDiscardsResultsOnReadOrCloseFailure(t *testing.T) {
	for _, scenario := range []string{"read", "close"} {
		t.Run(scenario, func(t *testing.T) {
			repo, db := retainedInventoryFixture(t)
			err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
				fault := retainedInventoryFaultTx{ReadTransaction: tx}
				if scenario == "read" {
					fault.nextErr = context.Canceled
				} else {
					fault.closeErr = errors.New("close failure")
				}
				got, err := repo.RetainedInventoryInTransaction(ctx, fault, RetainedInventoryLimits{MaxDocuments: 10, MaxBytes: 1 << 20})
				if err == nil || !reflect.DeepEqual(got, RetainedInventory{}) {
					t.Fatalf("failed scan returned inventory: %+v %v", got, err)
				}
				if scenario == "read" && !errors.Is(err, context.Canceled) {
					t.Fatalf("lost cancellation: %v", err)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

type retainedPhysicalKeyTx struct {
	dal.ReadTransaction
	parent  *record.Key
	numeric bool
}

func (tx retainedPhysicalKeyTx) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	reader, err := tx.ReadTransaction.ExecuteQueryToRecordsReader(ctx, q)
	if err != nil {
		return nil, err
	}
	return retainedPhysicalKeyReader{reader, tx.parent, tx.numeric}, nil
}

type retainedPhysicalKeyReader struct {
	dal.RecordsReader
	parent  *record.Key
	numeric bool
}

func (r retainedPhysicalKeyReader) Next() (record.Record, error) {
	row, err := r.RecordsReader.Next()
	if row == nil {
		return row, err
	}
	var id any = row.Key().ID
	if r.numeric {
		id = 123
	}
	return record.NewRecordWithData(record.NewKeyWithParentAndID(r.parent, row.Key().Collection(), id), row.Data()), err
}
func TestRetainedInventoryRejectsForeignPhysicalParentsAndCoercedKeys(t *testing.T) {
	for _, numeric := range []bool{false, true} {
		repo, db := retainedInventoryFixture(t)
		seedRetainedEnvelope(t, repo, Sessions, "123", "alice")
		parent := repo.parent
		if !numeric {
			parent = record.NewKeyWithParentAndID(parent.Parent(), parent.Collection(), "foreign-project")
		}
		err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
			inventory, err := repo.RetainedInventoryInTransaction(ctx, retainedPhysicalKeyTx{tx, parent, numeric}, RetainedInventoryLimits{MaxDocuments: 100, MaxBytes: 1 << 20})
			if err == nil || !reflect.DeepEqual(inventory, RetainedInventory{}) {
				t.Fatalf("foreign/coerced key accepted: %+v %v", inventory, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

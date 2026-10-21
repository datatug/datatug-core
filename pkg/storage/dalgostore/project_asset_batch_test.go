package dalgostore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
	"strings"
	"sync"
	"testing"
)

var errAssetBatchFault = errors.New("batch injected failure")

type guardedAssetTx struct {
	dal.ReadwriteTransaction
	writes, failWrite int
	queryFailure      bool
}

func (tx *guardedAssetTx) Get(ctx context.Context, r record.Record) error {
	if tx.writes > 0 {
		return errors.New("read after write")
	}
	return tx.ReadwriteTransaction.Get(ctx, r)
}
func (tx *guardedAssetTx) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	if tx.writes > 0 {
		return nil, errors.New("query after write")
	}
	if tx.queryFailure {
		return nil, errAssetBatchFault
	}
	return tx.ReadwriteTransaction.ExecuteQueryToRecordsReader(ctx, q)
}
func (tx *guardedAssetTx) Insert(ctx context.Context, r record.Record, opts ...dal.InsertOption) error {
	tx.writes++
	if tx.writes == tx.failWrite {
		return errAssetBatchFault
	}
	return tx.ReadwriteTransaction.Insert(ctx, r, opts...)
}

type guardedAssetDB struct {
	dal.DB
	failWrite    int
	queryFailure bool
	txs          int
}

func (db *guardedAssetDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, opts ...dal.TransactionOption) error {
	db.txs++
	return db.DB.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, &guardedAssetTx{ReadwriteTransaction: tx, failWrite: db.failWrite, queryFailure: db.queryFailure})
	}, opts...)
}

type onlyAssetSession struct{ dal.ReadwriteSession }
type onlyAssetReader struct{ dal.ReadSession }

func batchBoard(id string) *datatug.Board {
	b := internalAssetBoard(id)
	for _, kind := range []string{"timeline", "timing-bars", "heatmap"} {
		spec := datatug.ChartSpec{Type: kind, SelectionID: "evidence-" + kind}
		switch kind {
		case "timeline":
			spec.Timeline = &datatug.TimelineChartSpec{LaneColumn: "lane", EventColumn: "event", SequenceColumn: "seq", TimeColumn: "at", ClockDomainColumn: "clock", PrecisionColumn: "precision", StatusColumn: "status", EvidenceRefColumn: "evidence"}
		case "timing-bars":
			spec.Timing = &datatug.TimingChartSpec{RowKeyColumn: "key", LabelColumn: "label", StartTimeColumn: "start", EndTimeColumn: "end", StartClockDomainColumn: "clock", EndClockDomainColumn: "clock", StartPrecisionColumn: "precision", EndPrecisionColumn: "precision", StartEvidenceRefColumn: "start_ref", EndEvidenceRefColumn: "end_ref", StartIdentityColumn: "start_id", EndIdentityColumn: "end_id"}
		default:
			spec.Heatmap = &datatug.HeatmapChartSpec{RowColumn: "request", ColumnColumn: "expectation", StatusColumn: "status", EvidenceRefColumn: "evidence"}
		}
		b.Rows = append(b.Rows, datatug.BoardRow{Cards: datatug.BoardCards{{ID: kind, Widget: &datatug.BoardWidget{Name: "chart", Data: &datatug.ChartWidgetDef{Sources: []datatug.ChartSourceBinding{{ID: "held", QueryID: "shipping/events"}}, Chart: spec}}}}})
	}
	return b
}
func batchEntity(id string) *datatug.Entity {
	e := &datatug.Entity{Tables: datatug.TableKeys{datatug.NewTableKey("requests", "shipping_schema", "shipping", nil)}, Fields: datatug.EntityFields{{ID: "id", Type: "string", IsKeyField: true, Mappings: datatug.PhysicalRefs{{Source: "shipping", Collection: "requests", Column: "request_id"}}}}}
	e.ID = id
	return e
}
func requireBatchAbsent(t *testing.T, db dal.DB, root *record.Key, batch ProjectAssetBatch) {
	t.Helper()
	s, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	for _, b := range batch.Boards {
		_, err = s.LoadBoard(context.Background(), b.ID)
		require.True(t, record.IsNotFound(err), "%v", err)
	}
	for _, e := range batch.Entities {
		_, err = s.LoadEntity(context.Background(), e.ID)
		require.True(t, record.IsNotFound(err), "%v", err)
	}
}
func TestCreateAssetsActualTransactionRoundTripAndReadOrder(t *testing.T) {
	ctx := context.Background()
	db := &guardedAssetDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile())}
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "p")
	batch := ProjectAssetBatch{Boards: datatug.Boards{batchBoard("shared-id"), internalAssetBoard("second")}, Entities: datatug.Entities{batchEntity("shared-id")}}
	before, err := json.Marshal(batch)
	require.NoError(t, err)
	s, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	require.NoError(t, s.CreateAssets(ctx, batch))
	require.Equal(t, 1, db.txs)
	// Normal public loads, not a shortcut around the canonical stored codec.
	reopened, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	b, err := reopened.LoadBoard(ctx, "shared-id")
	require.NoError(t, err)
	second, err := reopened.LoadBoard(ctx, "second")
	require.NoError(t, err)
	e, err := reopened.LoadEntity(ctx, "shared-id")
	require.NoError(t, err)
	after, err := json.Marshal(ProjectAssetBatch{Boards: datatug.Boards{b, second}, Entities: datatug.Entities{e}})
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	require.Len(t, b.Rows, 3)
	require.Equal(t, "shipping_schema", e.Tables[0].Schema())
	require.Equal(t, "request_id", e.Fields[0].Mappings[0].Column)
	other, err := NewProjectItemsStore(db, "p", record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "two"), "projects", "p"))
	require.NoError(t, err)
	_, err = other.LoadBoard(ctx, "shared-id")
	require.True(t, record.IsNotFound(err))
	// Caller-owned tx has no nested transaction and can append a receipt after the batch.
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		items, err := NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		if err = items.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("third")}}); err != nil {
			return err
		}
		return tx.Insert(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(root, "receipts", "install"), &struct{ OK bool }{true}))
	}))
	require.Equal(t, 2, db.txs)
	// Native DB-backed store owns the same real atomic transaction.
	require.NoError(t, NewProjectStore(db, "native").CreateAssets(ctx, ProjectAssetBatch{Entities: datatug.Entities{batchEntity("entity")}}))
}
func TestCreateAssetsRollbackAtEveryWriteAndOuterAbort(t *testing.T) {
	ctx := context.Background()
	root := projectKey("p")
	batch := ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("b1"), internalAssetBoard("b2")}, Entities: datatug.Entities{batchEntity("e")}}
	for fail := 1; fail <= 3; fail++ {
		t.Run(fmt.Sprint(fail), func(t *testing.T) {
			db := &guardedAssetDB{DB: dalgo2memory.New(dalgo2memory.FirestoreProfile()), failWrite: fail}
			s, err := NewProjectItemsStore(db, "p", root)
			require.NoError(t, err)
			require.ErrorIs(t, s.CreateAssets(ctx, batch), errAssetBatchFault)
			requireBatchAbsent(t, db, root, batch)
		})
	}
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		s, err := NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		require.NoError(t, s.CreateAssets(ctx, batch))
		return errAssetBatchFault
	})
	require.ErrorIs(t, err, errAssetBatchFault)
	requireBatchAbsent(t, db, root, batch)
}
func TestCreateAssetsRejectsSessionsDefinitionsAndCollisionsBeforeWrites(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	root := projectKey("p")
	batch := ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("b")}}
	var nilDB *guardedAssetDB
	for _, session := range []dal.ReadSession{onlyAssetReader{db}, nilDB} {
		s, err := NewProjectItemsStore(session, "p", root)
		require.NoError(t, err)
		require.ErrorIs(t, s.CreateAssets(ctx, batch), ErrAssetReadOnly)
	}
	var nilStore *ProjectStore
	require.ErrorIs(t, nilStore.CreateAssets(ctx, batch), ErrAssetInvalid)
	s, err := NewProjectItemsStore(onlyAssetSession{db.(dal.ReadwriteSession)}, "p", root)
	require.NoError(t, err)
	require.ErrorIs(t, s.CreateAssets(ctx, batch), ErrAssetTransactionRequired)
	s, err = NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	for _, bad := range []ProjectAssetBatch{{}, {Boards: datatug.Boards{nil}}, {Entities: datatug.Entities{nil}}, {Boards: datatug.Boards{internalAssetBoard("../bad")}}, {Entities: datatug.Entities{batchEntity("../bad")}}, {Boards: datatug.Boards{&datatug.Board{}}}, {Entities: datatug.Entities{&datatug.Entity{Fields: datatug.EntityFields{{ID: "bad", Type: "invalid"}}}}}, {Boards: datatug.Boards{internalAssetBoard("same"), internalAssetBoard("same")}}} {
		require.Error(t, s.CreateAssets(ctx, bad))
		requireBatchAbsent(t, db, root, batch)
	}
	require.NoError(t, s.CreateAssets(ctx, batch))
	require.ErrorIs(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("new"), internalAssetBoard("b")}, Entities: datatug.Entities{batchEntity("e")}}), ErrAssetConflict)
	requireBatchAbsent(t, db, root, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("new")}, Entities: datatug.Entities{batchEntity("e")}})
	require.NoError(t, s.CreateAssets(ctx, ProjectAssetBatch{Entities: datatug.Entities{batchEntity("e")}}))
	require.ErrorIs(t, s.CreateAssets(ctx, ProjectAssetBatch{Entities: datatug.Entities{batchEntity("e")}}), ErrAssetConflict)
	// Even a matching definition conflicts; this API is never an upsert or receipt store.
	require.ErrorIs(t, s.CreateAssets(ctx, batch), ErrAssetConflict)
	require.Error(t, NewProjectStore(db, "bad/id").CreateAssets(ctx, batch))
}
func TestCreateAssetsCapacityBytesAndReadFailure(t *testing.T) {
	ctx := context.Background()
	root := projectKey("p")
	for _, kind := range []string{"boards", "entities"} {
		t.Run(kind, func(t *testing.T) {
			db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
			s, err := NewProjectItemsStore(db, "p", root)
			require.NoError(t, err)
			full := ProjectAssetBatch{}
			for i := 0; i < MaxProjectAssets; i++ {
				if kind == "boards" {
					full.Boards = append(full.Boards, internalAssetBoard(fmt.Sprintf("b%d", i)))
				} else {
					full.Entities = append(full.Entities, batchEntity(fmt.Sprintf("e%d", i)))
				}
			}
			require.NoError(t, s.CreateAssets(ctx, full))
			extra := ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("extra")}}
			if kind == "entities" {
				extra = ProjectAssetBatch{Entities: datatug.Entities{batchEntity("extra")}}
			}
			require.ErrorIs(t, s.CreateAssets(ctx, extra), ErrAssetLimit)
			requireBatchAbsent(t, db, root, extra)
			tooMany := full
			if kind == "boards" {
				tooMany.Boards = append(tooMany.Boards, internalAssetBoard("overflow"))
			} else {
				tooMany.Entities = append(tooMany.Entities, batchEntity("overflow"))
			}
			require.ErrorIs(t, s.CreateAssets(ctx, tooMany), ErrAssetLimit)
		})
	}
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	s, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	large := func(id string) *datatug.Board {
		b := internalAssetBoard(id)
		b.Rows = datatug.BoardRows{{Cards: datatug.BoardCards{{ID: "query", Widget: &datatug.BoardWidget{Name: "SQL", Data: datatug.SQLWidgetDef{SQL: datatug.SQLWidgetSettings{QueryID: "orders", Parameters: datatug.WidgetParameterBindings{{ID: "value", Value: strings.Repeat("x", MaxProjectAssetBytes-1000)}}}}}}}}}
		return b
	}
	require.ErrorIs(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{large("1"), large("2"), large("3"), large("4"), large("5"), large("6"), large("7"), large("8"), large("9")}}), ErrAssetLimit)
	require.NoError(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{large("1"), large("2"), large("3"), large("4"), large("5"), large("6"), large("7"), large("8")}}))
	require.ErrorIs(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{large("9")}}), ErrAssetLimit)
	tooBig := large("huge")
	tooBig.Rows[0].Cards[0].Widget.Data = datatug.SQLWidgetDef{SQL: datatug.SQLWidgetSettings{QueryID: "orders", Parameters: datatug.WidgetParameterBindings{{ID: "value", Value: strings.Repeat("x", MaxProjectAssetBytes+1)}}}}
	require.ErrorIs(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{tooBig}}), ErrAssetLimit)
	failing := &guardedAssetDB{DB: db, queryFailure: true}
	bad, err := NewProjectItemsStore(failing, "p", root)
	require.NoError(t, err)
	require.ErrorIs(t, bad.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("no-write")}}), errAssetBatchFault)
	requireBatchAbsent(t, db, root, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard("no-write")}})
	// A batch invoked after an earlier caller write fails before adding any asset.
	require.Error(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, raw dal.ReadwriteTransaction) error {
		tx := &guardedAssetTx{ReadwriteTransaction: raw}
		require.NoError(t, tx.Insert(ctx, record.NewRecordWithData(record.NewKeyWithID("other", "first"), &struct{ N int }{1})))
		items, err := NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		return items.CreateAssets(ctx, ProjectAssetBatch{Entities: datatug.Entities{batchEntity("late")}})
	}))
}

func TestCreateAssetsConcurrentCapacityAndActualReadonly(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	root := projectKey("p")
	store, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	initial := ProjectAssetBatch{}
	for i := 0; i < MaxProjectAssets-1; i++ {
		initial.Boards = append(initial.Boards, internalAssetBoard(fmt.Sprintf("b%d", i)))
	}
	require.NoError(t, store.CreateAssets(ctx, initial))
	start := make(chan struct{})
	outcomes := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{"concurrent-one", "concurrent-two"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			<-start
			outcomes <- store.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{internalAssetBoard(id)}})
		}(id)
	}
	close(start)
	wg.Wait()
	close(outcomes)
	successes, failures := 0, 0
	for err := range outcomes {
		if err == nil {
			successes++
		} else {
			require.ErrorIs(t, err, ErrAssetLimit)
			failures++
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures)
	boards, err := store.LoadBoards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, MaxProjectAssets)
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		reader, err := NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		require.ErrorIs(t, reader.CreateAssets(ctx, ProjectAssetBatch{Entities: datatug.Entities{batchEntity("no")}}), ErrAssetReadOnly)
		return nil
	}))
}

type mutateBeforeAssetTransactionDB struct {
	dal.DB
	mutate func()
}

func (db mutateBeforeAssetTransactionDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, opts ...dal.TransactionOption) error {
	db.mutate()
	return db.DB.RunReadwriteTransaction(ctx, f, opts...)
}
func TestCreateAssetsEncodesImmutableDefinitionsBeforeTransaction(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	board := internalAssetBoard("before")
	entity := batchEntity("entity")
	wrapped := mutateBeforeAssetTransactionDB{DB: db, mutate: func() {
		board.ID = "after"
		board.Title = "Mutated after validation"
		entity.Fields[0].Mappings[0].Column = "changed"
	}}
	s, err := NewProjectItemsStore(wrapped, "p", projectKey("p"))
	require.NoError(t, err)
	require.NoError(t, s.CreateAssets(ctx, ProjectAssetBatch{Boards: datatug.Boards{board}, Entities: datatug.Entities{entity}}))
	reopened, err := NewProjectItemsStore(db, "p", projectKey("p"))
	require.NoError(t, err)
	got, err := reopened.LoadBoard(ctx, "before")
	require.NoError(t, err)
	require.Equal(t, "Board", got.Title)
	_, err = reopened.LoadBoard(ctx, "after")
	require.True(t, record.IsNotFound(err))
	gotEntity, err := reopened.LoadEntity(ctx, "entity")
	require.NoError(t, err)
	require.Equal(t, "request_id", gotEntity.Fields[0].Mappings[0].Column)
}

package dalgostore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func internalAssetBoard(id string) *datatug.Board {
	b := &datatug.Board{}
	b.ID = id
	b.Title = "Board"
	return b
}
func TestAssetCodecRejectsInvalidStoredIdentityAndContent(t *testing.T) {
	for _, v := range []any{(*datatug.Board)(nil), (*datatug.Entity)(nil), "unsupported", &datatug.Board{}, &datatug.Entity{Fields: datatug.EntityFields{{ID: "x", Type: "invalid"}}}} {
		_, err := encodeAsset(v)
		require.Error(t, err)
	}
	b := internalAssetBoard("b")
	b.Rows = datatug.BoardRows{{Cards: datatug.BoardCards{{ID: "c", Widget: &datatug.BoardWidget{Name: "SQL", Data: &datatug.SQLWidgetDef{SQL: datatug.SQLWidgetSettings{QueryID: "q", Parameters: datatug.WidgetParameterBindings{{ID: "x", Value: make(chan int)}}}}}}}}}
	_, err := encodeAsset(b)
	require.Error(t, err)
	for _, body := range []string{"", strings.Repeat("x", MaxProjectAssetBytes+1), `[]`, `{"id":"other","title":"Board"}`, `{"id":"b","title":"Board"} {}`, `{"id":"b","unknown":1}`} {
		require.Error(t, decodeAsset([]byte(body), new(datatug.Board), "b"))
	}
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	root := projectKey("p")
	for _, input := range []struct {
		session dal.ReadSession
		id      string
		root    *record.Key
	}{{nil, "p", root}, {db, "", root}, {db, "p", nil}, {db, "other", root}, {db, "p", record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", int64(1)), "projects", "p")}} {
		_, err = NewProjectItemsStore(input.session, input.id, input.root)
		require.Error(t, err)
	}
	native := NewProjectStore(db, "invalid/id")
	require.Error(t, native.SaveBoard(context.Background(), internalAssetBoard("b")))
	require.Error(t, native.saveAsset(context.Background(), "boards", "invalid/id", internalAssetBoard("b")))
}

type assetFailureSession struct {
	dal.ReadwriteSession
	DB                       dal.DB
	getErr, setErr, queryErr bool
	records                  []record.Record
}

func (s assetFailureSession) Get(ctx context.Context, r record.Record) error {
	if s.getErr {
		return errors.New("asset read failed")
	}
	return s.DB.Get(ctx, r)
}
func (s assetFailureSession) Set(ctx context.Context, r record.Record) error {
	if s.setErr {
		return errors.New("asset write failed")
	}
	return s.ReadwriteSession.Set(ctx, r)
}
func (s assetFailureSession) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	if s.queryErr {
		return nil, errors.New("asset query failed")
	}
	if s.records != nil {
		return &assetResultReader{records: s.records}, nil
	}
	return s.DB.ExecuteQueryToRecordsReader(ctx, q)
}

type assetResultReader struct {
	records []record.Record
	i       int
}

func (r *assetResultReader) Next() (record.Record, error) {
	if r.i >= len(r.records) {
		return nil, io.EOF
	}
	v := r.records[r.i]
	r.i++
	return v, nil
}
func (*assetResultReader) Close() error            { return nil }
func (*assetResultReader) Cursor() (string, error) { return "", nil }
func TestAssetProviderFailuresAndMalformedListsFailClosed(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	root := projectKey("p")
	for _, fault := range []assetFailureSession{{ReadwriteSession: db.(dal.ReadwriteSession), DB: db, getErr: true}, {ReadwriteSession: db.(dal.ReadwriteSession), DB: db, setErr: true}, {ReadwriteSession: db.(dal.ReadwriteSession), DB: db, queryErr: true}} {
		s, err := NewProjectItemsStore(fault, "p", root)
		require.NoError(t, err)
		require.Error(t, s.SaveBoard(ctx, internalAssetBoard("b")))
		if fault.queryErr {
			_, err = s.LoadBoards(ctx)
			require.Error(t, err)
		}
	}
	docs := func(n int, size int) []record.Record {
		out := []record.Record{}
		for i := 0; i < n; i++ {
			id := fmt.Sprintf("b%03d", i)
			b := internalAssetBoard(id)
			body, _ := json.Marshal(b)
			if size > 0 {
				body = []byte(`{"id":"` + id + `","title":"Board","rows":[{"cards":[{"id":"c","widget":{"name":"SQL","data":{"sql":{"queryId":"q","parameters":[{"id":"p","value":"` + strings.Repeat("x", size) + `"}]}}}}]}]}`)
			}
			out = append(out, record.NewRecordWithData(record.NewKeyWithParentAndID(root, "boards", id), &assetDocument{Content: body}))
		}
		return out
	}
	for name, rows := range map[string][]record.Record{"overflow": docs(101, 0), "aggregate": docs(9, 240000), "bad-id": {record.NewRecordWithData(record.NewKeyWithParentAndID(root, "boards", int64(1)), &assetDocument{})}, "bad-type": {record.NewRecordWithData(record.NewKeyWithParentAndID(root, "boards", "b"), &struct{ Wrong string }{"wrong"})}, "bad-content": {record.NewRecordWithData(record.NewKeyWithParentAndID(root, "boards", "b"), &assetDocument{Content: []byte(`[]`)})}} {
		t.Run(name, func(t *testing.T) {
			s, err := NewProjectItemsStore(assetFailureSession{ReadwriteSession: db.(dal.ReadwriteSession), DB: db, records: rows}, "p", root)
			require.NoError(t, err)
			boards, err := s.LoadBoards(ctx)
			require.Error(t, err)
			require.Nil(t, boards)
			entities, err := s.LoadEntities(ctx)
			require.Error(t, err)
			require.Nil(t, entities)
		})
	}
	s, err := NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	for _, id := range []string{"One", "Two"} {
		e := &datatug.Entity{}
		e.ID = id
		require.NoError(t, s.SaveEntity(ctx, e))
	}
	// A source query/capacity failure before insert must never create a new item.
	failing, err := NewProjectItemsStore(assetFailureSession{ReadwriteSession: db.(dal.ReadwriteSession), DB: db, queryErr: true}, "p", root)
	require.NoError(t, err)
	require.Error(t, failing.SaveEntity(ctx, &datatug.Entity{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "Three"}}}))
}

type assetDeleteFaultDB struct {
	dal.DB
	kind string
}

func (db assetDeleteFaultDB) RunReadwriteTransaction(ctx context.Context, f dal.RWTxWorker, opts ...dal.TransactionOption) error {
	return db.DB.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return f(ctx, assetDeleteFaultTx{ReadwriteTransaction: tx, kind: db.kind})
	}, opts...)
}

type assetDeleteFaultTx struct {
	dal.ReadwriteTransaction
	kind string
}

func (tx assetDeleteFaultTx) ExecuteQueryToRecordsReader(ctx context.Context, q dal.Query) (dal.RecordsReader, error) {
	if q.(dal.StructuredQuery).From().Base().Name() == tx.kind {
		return nil, errors.New("delete admission query failed")
	}
	return tx.ReadwriteTransaction.ExecuteQueryToRecordsReader(ctx, q)
}
func TestNativeDeleteNeverIgnoresAssetReadFailure(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	for _, kind := range []string{"boards", "entities"} {
		require.Error(t, NewStore(assetDeleteFaultDB{DB: db, kind: kind}, "native").DeleteProject(ctx, "p"))
	}
	require.Error(t, NewStore(db, "native").DeleteProject(ctx, "unsafe/id"))
}

func TestLoadBoardRejectsCorruptStoredMetadataContent(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	store := NewProjectStore(db, "p")
	key, err := assetKey(projectKey("p"), "boards", "b")
	require.NoError(t, err)
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(key, &assetDocument{Content: []byte("{")}))
	}))
	_, err = store.LoadBoard(ctx, "b")
	require.ErrorIs(t, err, ErrAssetInvalid)
}

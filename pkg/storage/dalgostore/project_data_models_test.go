package dalgostore_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type countedDB struct {
	dal.DB
	sets, gets int
	readError  error
}
type countedTx struct {
	dal.ReadwriteTransaction
	db *countedDB
}

func (d *countedDB) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, options ...dal.TransactionOption) error {
	return d.DB.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error { return worker(ctx, &countedTx{tx, d}) }, options...)
}
func (t *countedTx) Get(ctx context.Context, r record.Record) error {
	t.db.gets++
	if t.db.readError != nil {
		return t.db.readError
	}
	return t.ReadwriteTransaction.Get(ctx, r)
}
func (t *countedTx) Set(ctx context.Context, r record.Record) error {
	t.db.sets++
	return t.ReadwriteTransaction.Set(ctx, r)
}
func modelDoc() *datatug.ProjectDataModels {
	return &datatug.ProjectDataModels{Version: 1, References: []datatug.ProjectDataModelReference{{ID: "m", DataModelReference: datatug.DataModelReference{Address: "modelspec://github.com/example/models/m", Commit: strings.Repeat("a", 40)}}}}
}
func TestDataModelsGenericDALGOExplicitRefusalPreservesState(t *testing.T) {
	ctx := context.Background()
	key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", "p")
	for _, state := range []string{`{"version":1,"references":[]}`, `{"version":2,"references":[]}`, `null`} {
		for _, incoming := range []*datatug.ProjectDataModels{nil, {Version: 1}, modelDoc()} {
			t.Run(state, func(t *testing.T) {
				db := &countedDB{DB: newTestDB()}
				store := dalgostore.NewProjectStore(db, "p")
				p := projectWithAccess(store, "p", "private")
				base, err := json.Marshal(datatug.ProjectFile{ProjectItem: p.ProjectItem, Created: p.Created})
				require.NoError(t, err)
				var raw map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(base, &raw))
				raw["dataModels"] = json.RawMessage(state)
				raw["future"] = json.RawMessage(`{"big":900719925474099312345678901}`)
				body, err := json.Marshal(raw)
				require.NoError(t, err)
				seed := json.RawMessage(body)
				setter, ok := db.DB.(interface {
					Set(context.Context, record.Record) error
				})
				require.True(t, ok)
				require.NoError(t, setter.Set(ctx, record.NewRecordWithData(key, &seed)))
				var before json.RawMessage
				require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &before)))
				p.DataModels = incoming
				p.Title = "must not persist"
				err = store.SaveProject(ctx, p)
				require.Error(t, err)
				require.Zero(t, db.sets)
				require.Greater(t, db.gets, 0, "existing state must be read inside write transaction")
				var after json.RawMessage
				require.NoError(t, db.Get(ctx, record.NewRecordWithData(key, &after)))
				require.Equal(t, string(before), string(after))
				_, err = store.LoadProjectFile(ctx)
				require.Error(t, err)
				_, err = store.LoadProject(ctx, datatug.Depth(1))
				require.Error(t, err)
			})
		}
	}
	db := &countedDB{DB: newTestDB()}
	store := dalgostore.NewProjectStore(db, "p")
	p := projectWithAccess(store, "p", "private")
	p.DataModels = modelDoc()
	require.ErrorIs(t, store.SaveProject(ctx, p), dalgostore.ErrDataModelsUnsupported)
	require.Zero(t, db.sets)
	db.readError = errors.New("read failed")
	p.DataModels = nil
	require.ErrorContains(t, store.SaveProject(ctx, p), "read failed")
	require.Zero(t, db.sets)
}
func TestDataModelsDALGOLegacyNoReferenceWritesRemainSupported(t *testing.T) {
	db := &countedDB{DB: newTestDB()}
	store := dalgostore.NewProjectStore(db, "p")
	p := projectWithAccess(store, "p", "public")
	require.NoError(t, store.SaveProject(context.Background(), p))
	require.Equal(t, 1, db.sets)
	file, err := store.LoadProjectFile(context.Background())
	require.NoError(t, err)
	require.Nil(t, file.DataModels)
}

// Exercise the real JSON-honoring memory provider already used by this package.
// The provider's typed ProjectFile read preserves opaque numeric tokens; a
// map[string]any intermediary would silently round this legacy metadata.
func TestDataModelsDALGOLegacyOpaqueNumericRead(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	key := record.NewKeyWithParentAndID(record.NewKeyWithID("ext", "datatug"), "projects", "p")
	const token = "90071992547409931234567890123456789"
	body := json.RawMessage(`{"id":"p","title":"Legacy","access":"private","future":{"number":` + token + `}}`)
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Set(ctx, record.NewRecordWithData(key, &body))
	}))
	file, err := dalgostore.NewProjectStore(db, "p").LoadProjectFile(ctx)
	require.NoError(t, err)
	encoded, err := json.Marshal(file)
	require.NoError(t, err)
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.JSONEq(t, `{"number":`+token+`}`, string(fields["future"]))
	require.Contains(t, string(fields["future"]), token)
}

// The wrapper changes only the independent namespace inspection result. This
// models provider errors or a changed record after the native typed read.
type namespaceInspectionDB struct {
	dal.DB
	reads   int
	inspect func(record.Record) error
}

func (d *namespaceInspectionDB) Get(ctx context.Context, r record.Record) error {
	d.reads++
	if err := d.DB.Get(ctx, r); err != nil {
		return err
	}
	if d.reads == 2 {
		return d.inspect(r)
	}
	return nil
}
func TestDataModelsDALGOIndependentNamespaceInspectionFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		inspect func(record.Record) error
	}{
		{"provider cannot inspect namespace", func(record.Record) error { return errors.New("namespace unavailable") }},
		{"case variant appeared", func(r record.Record) error {
			m := r.Data().(*map[string]json.RawMessage)
			(*m)["DATAMODELS"] = json.RawMessage(`null`)
			return nil
		}},
		{"malformed opaque raw metadata", func(r record.Record) error {
			m := r.Data().(*map[string]json.RawMessage)
			(*m)["future"] = json.RawMessage(`[`)
			return nil
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := &namespaceInspectionDB{DB: newTestDB(), inspect: tc.inspect}
			store := dalgostore.NewProjectStore(db, "p")
			require.NoError(t, store.SaveProject(context.Background(), projectWithAccess(store, "p", "private")))
			_, err := store.LoadProjectFile(context.Background())
			require.Error(t, err)
			require.Equal(t, 2, db.reads)
		})
	}
}

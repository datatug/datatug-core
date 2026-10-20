package dalgostore_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
)

func assetBoard(id string) *datatug.Board {
	b := &datatug.Board{Parameters: datatug.Parameters{{ID: "order_id", Type: "integer", DefaultValue: json.Number("9007199254740993")}}, Rows: datatug.BoardRows{{Cards: datatug.BoardCards{{ID: "query", Widget: &datatug.BoardWidget{Name: "SQL", Data: datatug.SQLWidgetDef{SQL: datatug.SQLWidgetSettings{QueryID: "orders", Parameters: datatug.WidgetParameterBindings{{ID: "order_id", Value: json.Number("9007199254740993")}}}}}}}}}}
	b.ID = id
	b.Title = "Investigation"
	return b
}
func assetEntity(id string) *datatug.Entity {
	e := &datatug.Entity{Tables: datatug.TableKeys{datatug.NewTableKey("requests", "shipping_schema", "shipping", nil)}, Fields: datatug.EntityFields{{ID: "order_id", Type: "integer", IsKeyField: true, Mappings: datatug.PhysicalRefs{{Source: "shipping", Collection: "requests", Column: "order_id"}}}, {ID: "attempt_id", Type: "string", IsKeyField: true}}}
	e.ID = id
	return e
}
func TestProjectAssetsActualDriverRoundTripAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "project")
	store, err := dalgostore.NewProjectItemsStore(db, "project", root)
	require.NoError(t, err)
	b := assetBoard("orders")
	before, err := json.Marshal(b)
	require.NoError(t, err)
	require.NoError(t, store.SaveBoard(ctx, b))
	require.NoError(t, store.SaveEntity(ctx, assetEntity("Order")))
	reopened, err := dalgostore.NewProjectItemsStore(db, "project", root)
	require.NoError(t, err)
	got, err := reopened.LoadBoard(ctx, "orders")
	require.NoError(t, err)
	after, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
	require.Contains(t, string(after), "9007199254740993")
	entity, err := reopened.LoadEntity(ctx, "Order")
	require.NoError(t, err)
	require.Len(t, entity.Fields, 2)
	require.Equal(t, "requests", entity.Tables[0].Name())
	require.Equal(t, "shipping_schema", entity.Tables[0].Schema())
	require.Equal(t, "shipping", entity.Tables[0].Catalog())
	require.Equal(t, "shipping", entity.Fields[0].Mappings[0].Source)
	boards, err := reopened.LoadBoards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, 1)
	entities, err := reopened.LoadEntities(ctx)
	require.NoError(t, err)
	require.Len(t, entities, 1)
	other, err := dalgostore.NewProjectItemsStore(db, "project", record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "two"), "projects", "project"))
	require.NoError(t, err)
	otherBoards, err := other.LoadBoards(ctx)
	require.NoError(t, err)
	require.Empty(t, otherBoards)
	// Mutating the caller's root after construction cannot retarget this store.
	root.ID = "elsewhere"
	require.NoError(t, store.DeleteBoard(ctx, "orders"))
	require.NoError(t, store.DeleteEntity(ctx, "Order"))
	boards, err = reopened.LoadBoards(ctx)
	require.NoError(t, err)
	require.Empty(t, boards)
}
func TestProjectItemsTransactionAbortAndParentRefusal(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	root := record.NewKeyWithID("projects", "p")
	abort := errors.New("abort")
	err := db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		s, err := dalgostore.NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		require.NoError(t, s.SaveBoard(ctx, assetBoard("board")))
		require.NoError(t, tx.Insert(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(root, "receipts", "operation"), &struct{ Value string }{"receipt"})))
		return abort
	})
	require.ErrorIs(t, err, abort)
	s, err := dalgostore.NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	_, err = s.LoadBoard(ctx, "board")
	require.True(t, record.IsNotFound(err))
	_, err = s.LoadProjectFile(ctx)
	require.ErrorIs(t, err, dalgostore.ErrNotImplemented)
	_, err = s.LoadProject(ctx)
	require.ErrorIs(t, err, dalgostore.ErrNotImplemented)
	require.ErrorIs(t, s.SaveProject(ctx, nil), dalgostore.ErrNotImplemented)
	require.NoError(t, db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		ro, err := dalgostore.NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		require.ErrorIs(t, ro.SaveBoard(ctx, assetBoard("board")), dalgostore.ErrAssetReadOnly)
		require.ErrorIs(t, ro.DeleteEntity(ctx, "Order"), dalgostore.ErrAssetReadOnly)
		return nil
	}))
}
func TestNativeDeleteRefusesNonemptyAssetsAndCapacity(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	store := dalgostore.NewStore(db, "native")
	items := dalgostore.NewProjectStore(db, "p")
	require.NoError(t, items.SaveBoard(ctx, assetBoard("board")))
	require.ErrorIs(t, store.DeleteProject(ctx, "p"), dalgostore.ErrNotImplemented)
	require.NoError(t, items.DeleteBoard(ctx, "board"))
	require.NoError(t, items.SaveEntity(ctx, assetEntity("Order")))
	require.ErrorIs(t, store.DeleteProject(ctx, "p"), dalgostore.ErrNotImplemented)
	require.NoError(t, items.DeleteEntity(ctx, "Order"))
	require.NoError(t, store.DeleteProject(ctx, "p"))
	for i := 0; i < 100; i++ {
		require.NoError(t, items.SaveBoard(ctx, assetBoard(fmt.Sprintf("b%03d", i))))
	}
	require.ErrorIs(t, items.SaveBoard(ctx, assetBoard("overflow")), dalgostore.ErrAssetLimit)
	require.NoError(t, items.SaveBoard(ctx, assetBoard("b000")))
	boards, err := items.LoadBoards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, 100)
	require.Equal(t, "b000", boards[0].ID)
	b := assetBoard("huge")
	w := b.Rows[0].Cards[0].Widget.Data.(datatug.SQLWidgetDef)
	w.SQL.Parameters[0].Value = strings.Repeat("x", dalgostore.MaxProjectAssetBytes+1)
	b.Rows[0].Cards[0].Widget.Data = w
	require.Error(t, items.SaveBoard(ctx, b))
	require.ErrorIs(t, items.SaveBoard(ctx, nil), dalgostore.ErrAssetInvalid)
	require.ErrorIs(t, items.SaveEntity(ctx, nil), dalgostore.ErrAssetInvalid)
	require.ErrorIs(t, items.DeleteBoard(ctx, "../unsafe"), dalgostore.ErrAssetInvalid)
	_, err = items.LoadEntity(ctx, "../unsafe")
	require.ErrorIs(t, err, dalgostore.ErrAssetInvalid)
}

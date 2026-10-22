package dto

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func TestProjectAssetMutationConditionsAndRevision(t *testing.T) {
	condition := AssetWriteCondition{ProjectRef: ProjectRef{StoreID: "firestore", ProjectID: "p"}, Environment: "demo", OperationID: "op", IfNoneMatch: true}
	board := datatug.Board{}
	board.ID = "b"
	board.Title = "Board"
	entity := datatug.Entity{}
	entity.ID = "e"
	require.NoError(t, (SaveBoardRequest{AssetWriteCondition: condition, Board: board}).Validate())
	require.NoError(t, (SaveEntityRequest{AssetWriteCondition: condition, Entity: entity}).Validate())
	for _, mutate := range []func(*AssetWriteCondition){func(c *AssetWriteCondition) { c.ProjectID = "" }, func(c *AssetWriteCondition) { c.StoreID = "" }, func(c *AssetWriteCondition) { c.OperationID = "/unsafe" }, func(c *AssetWriteCondition) { c.Environment = "" }, func(c *AssetWriteCondition) { c.Environment = " demo" }, func(c *AssetWriteCondition) { c.IfNoneMatch = false }, func(c *AssetWriteCondition) { c.IfMatch = "revision" }} {
		invalid := condition
		mutate(&invalid)
		require.Error(t, (SaveBoardRequest{AssetWriteCondition: invalid, Board: board}).Validate())
		require.Error(t, (SaveEntityRequest{AssetWriteCondition: invalid, Entity: entity}).Validate())
	}
	require.Error(t, (SaveBoardRequest{AssetWriteCondition: condition}).Validate())
	bad := entity
	bad.Fields = datatug.EntityFields{{ID: "field", Type: "unknown"}}
	require.Error(t, (SaveEntityRequest{AssetWriteCondition: condition, Entity: bad}).Validate())
	createRevision, err := ProjectAssetRevision("board", board)
	require.NoError(t, err)
	require.Contains(t, createRevision, "board1-")
	body, err := json.Marshal(board)
	require.NoError(t, err)
	var decoded datatug.Board
	require.NoError(t, json.Unmarshal(body, &decoded))
	same, err := ProjectAssetRevision("board", decoded)
	require.NoError(t, err)
	require.Equal(t, createRevision, same)
	other, err := ProjectAssetRevision("entity", entity)
	require.NoError(t, err)
	require.NotEqual(t, createRevision, other)
	_, err = ProjectAssetRevision("unknown", board)
	require.ErrorIs(t, err, ErrUnsupportedCapability)
	_, err = ProjectAssetRevision("board", make(chan int))
	require.Error(t, err)
}

func TestSaveCardRequestAndRevision(t *testing.T) {
	condition := AssetWriteCondition{ProjectRef: ProjectRef{StoreID: "firestore", ProjectID: "p"}, Environment: "demo", OperationID: "op", IfNoneMatch: true}
	card := datatug.Card{ID: "c", Title: "Card", Widget: &datatug.BoardWidget{Name: "text", Data: &datatug.TextWidgetDef{Markdown: "x"}}}
	require.NoError(t, (SaveCardRequest{AssetWriteCondition: condition, Card: card}).Validate())
	invalid := condition
	invalid.Environment = ""
	require.Error(t, (SaveCardRequest{AssetWriteCondition: invalid, Card: card}).Validate())
	require.Error(t, (SaveCardRequest{AssetWriteCondition: condition}).Validate())
	revision, err := ProjectAssetRevision("card", card)
	require.NoError(t, err)
	require.Contains(t, revision, "card1-")
}

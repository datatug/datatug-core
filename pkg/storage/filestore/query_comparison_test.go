package filestore

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func TestComparisonRevisionedQueryRoundTripAndCAS(t *testing.T) {
	data, err := os.ReadFile("../../datatug/testdata/query_comparison.json")
	require.NoError(t, err)
	var q datatug.QueryDefWithFolderPath
	require.NoError(t, json.Unmarshal(data, &q))
	q.FolderPath = "compare"
	store := NewProjectStore("test", t.TempDir()).(datatug.RevisionedQueriesStore)
	first, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	require.NoError(t, err)
	loaded, err := store.LoadQueryRevision(context.Background(), "compare/pair")
	require.NoError(t, err)
	require.Equal(t, q.Comparison, loaded.Query.Comparison)
	require.Equal(t, q.Recordsets, loaded.Query.Recordsets)
	// A valid projection alias edit belongs to the exact metadata revision.
	q.Recordsets[0].Columns[2].Name = "new_lane"
	q.Comparison.Projection.LaneColumn = "new_lane"
	second, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
	require.NoError(t, err)
	require.NotEqual(t, first.Revision, second.Revision)
	require.Equal(t, first.Query.CreatedAt, second.Query.CreatedAt)
	_, err = store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
	require.Error(t, err)
	q.Comparison.Projection.AnchorColumn = "Password=secret"
	_, err = store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfMatch: second.Revision})
	require.Error(t, err)
	final, err := store.LoadQueryRevision(context.Background(), "compare/pair")
	require.NoError(t, err)
	require.Equal(t, second.Revision, final.Revision)
}

func TestElapsedTimelineActualStoreRoundTrip(t *testing.T) {
	data, err := os.ReadFile("../../datatug/testdata/timeline_elapsed.json")
	require.NoError(t, err)
	var timeline datatug.TimelineChartSpec
	require.NoError(t, json.Unmarshal(data, &timeline))
	chart := datatug.ChartWidgetDef{Sources: []datatug.ChartSourceBinding{{ID: "held", QueryID: "pair"}}, Chart: datatug.ChartSpec{Type: "timeline", SelectionID: "observed", Timeline: &timeline}}
	board := &datatug.Board{ProjectItem: datatug.ProjectItem{ID: "elapsed", Title: "Observed elapsed"}, Rows: datatug.BoardRows{{Cards: datatug.BoardCards{{ID: "events", Widget: &datatug.BoardWidget{Name: "chart", Data: &chart}}}}}}
	store := newFsProjectStore("test", t.TempDir())
	require.NoError(t, store.SaveBoard(context.Background(), board))
	loaded, err := store.LoadBoard(context.Background(), board.ID)
	require.NoError(t, err)
	require.NoError(t, loaded.Validate())
	before, err := json.Marshal(board)
	require.NoError(t, err)
	after, err := json.Marshal(loaded)
	require.NoError(t, err)
	require.JSONEq(t, string(before), string(after))
}

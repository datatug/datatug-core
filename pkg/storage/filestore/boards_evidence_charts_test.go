package filestore

import (
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEvidenceChartsActualProjectStoreRoundTrip(t *testing.T) {
	for _, kind := range []string{"timeline", "heatmap"} {
		t.Run(kind, func(t *testing.T) {
			spec := datatug.ChartSpec{Type: kind, SelectionID: "evidence"}
			if kind == "timeline" {
				spec.Timeline = &datatug.TimelineChartSpec{LaneColumn: "lane", EventColumn: "event", SequenceColumn: "seq", TimeColumn: "at", ClockDomainColumn: "clock", PrecisionColumn: "precision", StatusColumn: "status", EvidenceRefColumn: "evidence"}
			} else {
				spec.Heatmap = &datatug.HeatmapChartSpec{RowColumn: "request", ColumnColumn: "expectation", StatusColumn: "status", EvidenceRefColumn: "evidence"}
			}
			board := &datatug.Board{ProjectItem: datatug.ProjectItem{ID: "processing", Title: "Processing"}, Rows: datatug.BoardRows{{Cards: datatug.BoardCards{{ID: "process", Widget: &datatug.BoardWidget{Name: "chart", Data: &datatug.ChartWidgetDef{Sources: []datatug.ChartSourceBinding{{ID: "held", QueryID: "shipping/process"}}, Chart: spec}}}}}}}
			require.NoError(t, board.Validate())
			store := newFsProjectStore("training", t.TempDir())
			require.NoError(t, store.SaveBoard(context.Background(), board))
			loaded, err := store.LoadBoard(context.Background(), board.ID)
			require.NoError(t, err)
			require.NoError(t, loaded.Validate())
			before, err := json.Marshal(board)
			require.NoError(t, err)
			after, err := json.Marshal(loaded)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
		})
	}
}

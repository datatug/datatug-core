package datatug

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func evidenceChart(kind string) ChartWidgetDef {
	spec := ChartSpec{Type: kind, SelectionID: "evidence"}
	if kind == "timeline" {
		spec.Timeline = &TimelineChartSpec{LaneColumn: "attempt", EventColumn: "stage", SequenceColumn: "seq", TimeColumn: "at", ClockDomainColumn: "clock", PrecisionColumn: "precision", StatusColumn: "status", EvidenceRefColumn: "evidence"}
	} else {
		spec.Heatmap = &HeatmapChartSpec{RowColumn: "request", ColumnColumn: "expectation", StatusColumn: "outcome", EvidenceRefColumn: "evidence"}
	}
	return ChartWidgetDef{Sources: []ChartSourceBinding{{ID: "held", QueryID: "shipping/events"}}, Chart: spec}
}
func TestEvidenceChartWireRoundTrip(t *testing.T) {
	for _, kind := range []string{"timeline", "heatmap"} {
		t.Run(kind, func(t *testing.T) {
			widget := evidenceChart(kind)
			require.NoError(t, widget.Validate())
			data, err := json.Marshal(widget)
			require.NoError(t, err)
			var decoded ChartWidgetDef
			require.NoError(t, json.Unmarshal(data, &decoded))
			require.NoError(t, decoded.Validate())
			require.Equal(t, widget, decoded)
			board := linkedBoard()
			board.Rows[0].Cards[0].Widget = &BoardWidget{Name: "chart", Data: widget} // Remove unrelated linked filter for this standalone chart.
			board.Rows = board.Rows[:1]
			require.NoError(t, board.Validate())
		})
	}
}
func TestEvidenceChartsRejectUnsupportedBindings(t *testing.T) {
	for _, kind := range []string{"timeline", "heatmap"} {
		cases := map[string]func(*ChartWidgetDef){"multiple results": func(w *ChartWidgetDef) {
			w.Sources = append(w.Sources, ChartSourceBinding{ID: "other", QueryID: "other"})
		}, "selection missing": func(w *ChartWidgetDef) { w.Chart.SelectionID = "" }, "bar fields": func(w *ChartWidgetDef) { w.Chart.ValueColumn = "invented" }, "inline source": func(w *ChartWidgetDef) { w.Sources[0].QueryID = "" }, "mixed variant": func(w *ChartWidgetDef) {
			if kind == "timeline" {
				w.Chart.Heatmap = &HeatmapChartSpec{}
			} else {
				w.Chart.Timeline = &TimelineChartSpec{}
			}
		}, "duplicate column": func(w *ChartWidgetDef) {
			if w.Chart.Timeline != nil {
				w.Chart.Timeline.TimeColumn = w.Chart.Timeline.EventColumn
			} else {
				w.Chart.Heatmap.ColumnColumn = w.Chart.Heatmap.RowColumn
			}
		}, "source transform": func(w *ChartWidgetDef) {
			w.Sources[0].Transform = &ChartSourceTransform{Name: "orders-monthly-country-count", OrderIDColumn: "id", DateColumn: "date", CountryColumn: "country"}
		}}
		for name, mutate := range cases {
			t.Run(kind+"/"+name, func(t *testing.T) { w := evidenceChart(kind); mutate(&w); require.Error(t, w.Validate()) })
		}
	}
}
func TestEvidenceChartUnknownWireFields(t *testing.T) {
	for _, wire := range []string{`{"type":"timeline","timeline":{"laneColumn":"x","sql":"select secret"}}`, `{"type":"heatmap","Heatmap":{}}`, `{"type":"timeline","valueColumn":null}`, `{"type":"heatmap","ValueColumn":""}`, `{"type":"heatmap","heatmap":{"rowColumn":"a","rows":[1]}}`} {
		var spec ChartSpec
		require.Error(t, json.Unmarshal([]byte(wire), &spec))
	}
}

func TestEvidenceChartMappingAdmissionBoundaries(t *testing.T) {
	timeline := evidenceChart("timeline")
	timeline.Chart.Timeline.TimeColumn = ""
	require.ErrorContains(t, timeline.Validate(), "nonempty")
	heatmap := evidenceChart("heatmap")
	heatmap.Chart.Heatmap.StatusColumn = " status"
	require.ErrorContains(t, heatmap.Validate(), "canonical")
	legacy := comparisonChart()
	legacy.Chart.Timeline = &TimelineChartSpec{}
	require.ErrorContains(t, legacy.Validate(), "matching type")
	// The internal validator fails closed if a future caller bypasses dispatch.
	require.ErrorContains(t, (ChartSpec{Type: "unsupported", SelectionID: "held"}).validateEvidenceChart(), "unknown evidence chart")
}

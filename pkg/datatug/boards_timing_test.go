package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func timingSpec() *TimingChartSpec {
	return &TimingChartSpec{RowKeyColumn: "row_key", LabelColumn: "stage", StartTimeColumn: "start_at", EndTimeColumn: "end_at", StartClockDomainColumn: "clock", EndClockDomainColumn: "clock", StartPrecisionColumn: "precision", EndPrecisionColumn: "precision", StartEvidenceRefColumn: "start_evidence", EndEvidenceRefColumn: "end_evidence", StartIdentityColumn: "start_event", EndIdentityColumn: "end_event"}
}
func timingWidget() ChartWidgetDef {
	return ChartWidgetDef{Sources: []ChartSourceBinding{{ID: "held", QueryID: "shipping/timings"}}, Chart: ChartSpec{Type: "timing-bars", SelectionID: "timing", Timing: timingSpec()}}
}
func TestTimingChartWire(t *testing.T) {
	w := timingWidget()
	require.NoError(t, w.Validate())
	raw, err := json.Marshal(w)
	require.NoError(t, err)
	var decoded ChartWidgetDef
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NoError(t, decoded.Validate())
	require.Equal(t, w, decoded)
	w.Chart.Timing.EndClockDomainColumn = "end_clock"
	w.Chart.Timing.EndPrecisionColumn = "end_precision"
	require.NoError(t, w.Validate())
}
func TestTimingChartRejectsInvalidMappingsAndBindings(t *testing.T) {
	for name, mutate := range map[string]func(*ChartWidgetDef){
		"missing timing": func(w *ChartWidgetDef) { w.Chart.Timing = nil }, "timeline": func(w *ChartWidgetDef) { w.Chart.Timeline = &TimelineChartSpec{} }, "heatmap": func(w *ChartWidgetDef) { w.Chart.Heatmap = &HeatmapChartSpec{} }, "missing identity": func(w *ChartWidgetDef) { w.Chart.Timing.StartIdentityColumn = "" }, "same identity columns": func(w *ChartWidgetDef) { w.Chart.Timing.EndIdentityColumn = w.Chart.Timing.StartIdentityColumn }, "whitespace": func(w *ChartWidgetDef) { w.Chart.Timing.LabelColumn = " stage" }, "missing end clock": func(w *ChartWidgetDef) { w.Chart.Timing.EndClockDomainColumn = "" }, "missing end precision": func(w *ChartWidgetDef) { w.Chart.Timing.EndPrecisionColumn = "" }, "multiple sources": func(w *ChartWidgetDef) { w.Sources = append(w.Sources, w.Sources[0]) }, "transform": func(w *ChartWidgetDef) { w.Sources[0].Transform = &ChartSourceTransform{} }, "bar option": func(w *ChartWidgetDef) { w.Chart.Unit = "ms" }, "wrong kind": func(w *ChartWidgetDef) { w.Chart.Type = "bar" },
	} {
		t.Run(name, func(t *testing.T) { w := timingWidget(); mutate(&w); require.Error(t, w.Validate()) })
	}
	for _, kind := range []string{"timeline", "heatmap"} {
		w := evidenceChart(kind)
		w.Chart.Timing = timingSpec()
		require.Error(t, w.Validate())
	}
}
func TestTimingChartStrictWire(t *testing.T) {
	for _, raw := range []string{`{"type":"timing-bars","timing":{"startIdentityColumn":"a","startIdentityColumn":"b"}}`, `{"type":"timing-bars","timing":{"sql":"select 1"}}`, `{"type":"timing-bars","Timing":{}}`, `{"type":"timing-bars","timing":{"StartTimeColumn":"x"}}`, `{"type":"timing-bars","timing":{"startTimeColumn":null}}`, `{"type":"timing-bars","timing":{"startTimeColumn":1}}`, `{"type":"timing-bars","valueColumn":null}`, `{"type":"timing-bars","timing":[]}`, `{"type":"timing-bars","type":"bar"}`} {
		var s ChartSpec
		require.Error(t, json.Unmarshal([]byte(raw), &s))
	}
	var s TimingChartSpec
	require.Error(t, s.UnmarshalJSON([]byte(`{"`)))
}

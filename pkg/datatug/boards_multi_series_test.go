package datatug

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const savedLinkedComparisonBoardJSON = `{"id":"showcase","title":"Showcase","rows":[{"cards":[{"id":"comparison","title":"Customer countries","widget":{"name":"chart","data":{"sources":[{"id":"comparison","queryId":"demodb/customer-country-comparison-server"}],"chart":{"type":"bar","categoryColumn":"country","unit":"customer records","series":[{"id":"northwind","title":"Northwind","valueColumn":"northwindCustomers"},{"id":"chinook","title":"Chinook","valueColumn":"chinookCustomers"}],"selectionId":"customer-country"}}}}]},{"cards":[{"id":"orders","title":"Orders shipped to","widget":{"name":"SQL","data":{"sql":{"queryId":"demodb/northwind-orders-full-sqlite","presentationFilter":{"selectionId":"customer-country","column":"ShipCountry"}}}}}]}]}`

func comparisonChart() ChartWidgetDef {
	return ChartWidgetDef{
		Sources: []ChartSourceBinding{{ID: "comparison", QueryID: "demodb/customer-country-comparison-server"}},
		Chart: ChartSpec{
			Type:           "bar",
			CategoryColumn: "country",
			Unit:           "customer records",
			SelectionID:    "customer-country",
			Series: []ChartSeries{
				{ID: "northwind", Title: "Northwind", ValueColumn: "northwindCustomers"},
				{ID: "chinook", Title: "Chinook", ValueColumn: "chinookCustomers"},
			},
		},
	}
}

func linkedBoard() Board {
	chart := comparisonChart()
	return Board{
		ProjectItem: ProjectItem{ID: "showcase", Title: "Showcase"},
		Rows: BoardRows{
			{Cards: BoardCards{{ID: "chart", Widget: &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{
				{ID: "placeholder", Title: "Empty"},
				{ID: "comparison", Title: "Compare", Widget: BoardWidget{Name: "chart", Data: &chart}},
			}}}}}},
			{Cards: BoardCards{{ID: "orders", Widget: &BoardWidget{Name: string(QueryTypeSQL), Data: &SQLWidgetDef{SQL: SQLWidgetSettings{
				QueryID: "demodb/northwind-orders-full-sqlite",
				PresentationFilter: &PresentationFilter{
					SelectionID: "customer-country",
					Column:      "ShipCountry",
				},
			}}}}}},
		},
	}
}

func TestMultiSeriesBoardRoundTripAndLinks(t *testing.T) {
	typed := linkedBoard()
	require.NoError(t, typed.Validate())
	encoded, err := json.Marshal(typed)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), `"series":[`)
	assert.NotContains(t, string(encoded), `"valueColumn":""`)

	var raw Board
	require.NoError(t, json.Unmarshal(encoded, &raw))
	require.NoError(t, raw.Validate())
	roundTrip, err := json.Marshal(raw)
	require.NoError(t, err)
	var before, after any
	require.NoError(t, json.Unmarshal(encoded, &before))
	require.NoError(t, json.Unmarshal(roundTrip, &after))
	assert.Equal(t, before, after)

	// A producer in an inactive tab still resolves a detail in another card.
	tabs := typed.Rows[0].Cards[0].Widget.Data.(*TabsWidgetDef)
	tabs.Tabs[1], tabs.Tabs[0] = tabs.Tabs[0], tabs.Tabs[1]
	require.NoError(t, typed.Validate())
}

func TestSavedLinkedComparisonBoardWire(t *testing.T) {
	var board Board
	require.NoError(t, json.Unmarshal([]byte(savedLinkedComparisonBoardJSON), &board))
	require.NoError(t, board.Validate())
	encoded, err := json.Marshal(board)
	require.NoError(t, err)
	var before, after any
	require.NoError(t, json.Unmarshal([]byte(savedLinkedComparisonBoardJSON), &before))
	require.NoError(t, json.Unmarshal(encoded, &after))
	assert.Equal(t, before, after)
}

func TestMultiSeriesChartBoundaries(t *testing.T) {
	valid := comparisonChart()
	require.NoError(t, valid.Validate())
	for _, n := range []int{2, 8} {
		t.Run(fmt.Sprintf("%d series", n), func(t *testing.T) {
			chart := comparisonChart()
			for len(chart.Chart.Series) < n {
				i := len(chart.Chart.Series)
				chart.Chart.Series = append(chart.Chart.Series, ChartSeries{
					ID: fmt.Sprintf("source%d", i), Title: fmt.Sprintf("Source %d", i), ValueColumn: fmt.Sprintf("measure%d", i),
				})
			}
			require.NoError(t, chart.Validate())
		})
	}
	cases := []struct {
		name string
		edit func(*ChartWidgetDef)
		want string
	}{
		{"one series", func(v *ChartWidgetDef) { v.Chart.Series = v.Chart.Series[:1] }, "2 to 8"},
		{"nine series", func(v *ChartWidgetDef) {
			for len(v.Chart.Series) < 9 {
				i := len(v.Chart.Series)
				v.Chart.Series = append(v.Chart.Series, ChartSeries{ID: fmt.Sprint(i), Title: "More", ValueColumn: fmt.Sprint("measure", i)})
			}
		}, "2 to 8"},
		{"missing unit", func(v *ChartWidgetDef) { v.Chart.Unit = "" }, "unit"},
		{"mixed legacy measure", func(v *ChartWidgetDef) { v.Chart.ValueColumn = "orders" }, "mutually exclusive"},
		{"duplicate id", func(v *ChartWidgetDef) { v.Chart.Series[1].ID = "northwind" }, "duplicate id"},
		{"padded id", func(v *ChartWidgetDef) { v.Chart.Series[1].ID = " chinook " }, "whitespace"},
		{"duplicate measure", func(v *ChartWidgetDef) { v.Chart.Series[1].ValueColumn = "northwindCustomers" }, "duplicate valueColumn"},
		{"category as measure", func(v *ChartWidgetDef) { v.Chart.Series[1].ValueColumn = "country" }, "must differ"},
		{"empty title", func(v *ChartWidgetDef) { v.Chart.Series[1].Title = " " }, "title"},
		{"multiple sources", func(v *ChartWidgetDef) { v.Sources = append(v.Sources, ChartSourceBinding{ID: "other", QueryID: "q"}) }, "one source"},
		{"source transform", func(v *ChartWidgetDef) {
			v.Sources[0].Transform = &ChartSourceTransform{Name: "orders-monthly-country-count", OrderIDColumn: "OrderID", DateColumn: "OrderDate", CountryColumn: "ShipCountry"}
		}, "source transform"},
		{"race", func(v *ChartWidgetDef) { v.Chart.Type = "bar-race" }, "require bar"},
		{"padded selection", func(v *ChartWidgetDef) { v.Chart.SelectionID = " country " }, "selectionId"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			chart := comparisonChart()
			tc.edit(&chart)
			assert.ErrorContains(t, chart.Validate(), tc.want)
		})
	}
}

func TestMultiSeriesRawFieldPresenceAndClosedFilter(t *testing.T) {
	validSpec := `{"type":"bar","categoryColumn":"country","unit":"customer records","series":[{"id":"northwind","title":"Northwind","valueColumn":"northwindCustomers"},{"id":"chinook","title":"Chinook","valueColumn":"chinookCustomers"}],"selectionId":"customer-country"}`
	for _, tc := range []struct {
		name, spec, want string
	}{
		{"valid", validSpec, ""},
		{"empty legacy field", strings.Replace(validSpec, `"series":`, `"valueColumn":"","series":`, 1), "omitted"},
		{"null legacy field", strings.Replace(validSpec, `"series":`, `"valueColumn":null,"series":`, 1), "omitted"},
		{"null series", strings.Replace(validSpec, `"series":[{"id":"northwind","title":"Northwind","valueColumn":"northwindCustomers"},{"id":"chinook","title":"Chinook","valueColumn":"chinookCustomers"}]`, `"series":null`, 1), "series"},
		{"empty series", strings.Replace(validSpec, `"series":[{"id":"northwind","title":"Northwind","valueColumn":"northwindCustomers"},{"id":"chinook","title":"Chinook","valueColumn":"chinookCustomers"}]`, `"series":[]`, 1), "2 to 8"},
		{"null selection", strings.Replace(validSpec, `"selectionId":"customer-country"`, `"selectionId":null`, 1), "selectionId"},
		{"empty selection", strings.Replace(validSpec, `"selectionId":"customer-country"`, `"selectionId":""`, 1), "selectionId"},
		{"unknown series field", strings.Replace(validSpec, `"id":"northwind"`, `"id":"northwind","expression":"1+1"`, 1), "unknown field"},
		{"unknown chart field", strings.Replace(validSpec, `"selectionId":"customer-country"`, `"selectionId":"customer-country","rows":[]`, 1), "unknown field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var spec ChartSpec
			err := json.Unmarshal([]byte(tc.spec), &spec)
			if err == nil {
				err = spec.Validate()
			}
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
	for _, tc := range []struct{ name, filter, want string }{
		{"valid", `{"selectionId":"customer-country","column":"ShipCountry"}`, ""},
		{"null", `null`, "must not be null"},
		{"null column", `{"selectionId":"customer-country","column":null}`, "column"},
		{"empty id", `{"selectionId":"","column":"ShipCountry"}`, "selectionId"},
		{"unknown expression", `{"selectionId":"customer-country","column":"ShipCountry","expression":"1=1"}`, "unknown field"},
	} {
		t.Run("filter "+tc.name, func(t *testing.T) {
			var settings SQLWidgetSettings
			err := json.Unmarshal([]byte(`{"queryId":"orders","legacyUnknown":true,"presentationFilter":`+tc.filter+`}`), &settings)
			if err == nil {
				err = settings.Validate()
			}
			if tc.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.want)
			}
		})
	}
}

func TestBoardPresentationLinkFailures(t *testing.T) {
	t.Run("unresolved", func(t *testing.T) {
		board := linkedBoard()
		sql := board.Rows[1].Cards[0].Widget.Data.(*SQLWidgetDef)
		sql.SQL.PresentationFilter.SelectionID = "missing"
		assert.ErrorContains(t, board.Validate(), "unresolved selectionId")
	})
	t.Run("duplicate in another tab", func(t *testing.T) {
		board := linkedBoard()
		tabs := board.Rows[0].Cards[0].Widget.Data.(*TabsWidgetDef)
		duplicate := comparisonChart()
		tabs.Tabs = append(tabs.Tabs, TabWidget{ID: "other", Title: "Other", Widget: BoardWidget{Name: "chart", Data: &duplicate}})
		assert.ErrorContains(t, board.Validate(), "duplicate selectionId")
	})
	t.Run("raw map unresolved", func(t *testing.T) {
		board := linkedBoard()
		bytes, err := json.Marshal(board)
		require.NoError(t, err)
		var raw Board
		require.NoError(t, json.Unmarshal(bytes, &raw))
		tabs := raw.Rows[0].Cards[0].Widget.Data.(map[string]any)
		entries := tabs["tabs"].([]any)
		chart := entries[1].(map[string]any)["widget"].(map[string]any)["data"].(map[string]any)["chart"].(map[string]any)
		chart["selectionId"] = "changed"
		assert.ErrorContains(t, raw.Validate(), "unresolved selectionId")
	})
}

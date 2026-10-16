package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const savedChartBoardJSON = `{"id":"northwind","title":"Northwind","rows":[{"cards":[{"id":"countries","title":"Orders by country","widget":{"name":"tabs","data":{"tabs":[{"id":"comparison","title":"Compare sources","widget":{"name":"chart","data":{"sources":[{"id":"sqlite","queryId":"demodb/northwind-shipping-country-count-sqlite"},{"id":"postgresql","queryId":"demodb/northwind-shipping-country-count-postgresql"}],"chart":{"type":"bar","categoryColumn":"ShipCountry","valueColumn":"orders"},"defaultView":"chart"}}},{"id":"data","title":"Data","widget":{"name":"SQL","data":{"sql":{"queryId":"demodb/northwind-shipping-country-count-sqlite"}}}}]}}}]}]}`

func TestBoardChartWireRoundTrip(t *testing.T) {
	var board Board
	require.NoError(t, json.Unmarshal([]byte(savedChartBoardJSON), &board))
	require.NoError(t, board.Validate())

	encoded, err := json.Marshal(board)
	require.NoError(t, err)
	var decoded Board
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.NoError(t, decoded.Validate())
	var before, after any
	require.NoError(t, json.Unmarshal([]byte(savedChartBoardJSON), &before))
	require.NoError(t, json.Unmarshal(encoded, &after))
	assert.Equal(t, before, after)
	assert.NotContains(t, string(encoded), "execution")
	assert.NotContains(t, string(encoded), "rowsData")
}

func TestChartWidgetValidation(t *testing.T) {
	valid := func() ChartWidgetDef {
		return ChartWidgetDef{
			Sources:     []ChartSourceBinding{{ID: "sqlite", QueryID: "q1", Parameters: WidgetParameterBindings{{ID: "country"}}}},
			Chart:       ChartSpec{Type: "bar", CategoryColumn: "ShipCountry", ValueColumn: "orders"},
			DefaultView: "grid",
		}
	}
	t.Run("typed and JSON decoded", func(t *testing.T) {
		assert.NoError(t, (BoardWidget{Name: "chart", Data: valid()}).Validate())
		assert.NoError(t, (BoardWidget{Name: "chart", Data: map[string]any{
			"sources": []any{map[string]any{"id": "sqlite", "queryId": "q1"}},
			"chart":   map[string]any{"type": "bar", "categoryColumn": "ShipCountry", "valueColumn": "orders"},
		}}).Validate())
	})
	tests := []struct {
		name string
		edit func(*ChartWidgetDef)
		want string
	}{
		{"no sources", func(v *ChartWidgetDef) { v.Sources = nil }, "sources"},
		{"no query", func(v *ChartWidgetDef) { v.Sources[0].QueryID = "" }, "queryId"},
		{"duplicate source ID", func(v *ChartWidgetDef) {
			v.Sources = append(v.Sources, ChartSourceBinding{ID: "sqlite", QueryID: "q2"})
		}, "duplicate id"},
		{"bad binding", func(v *ChartWidgetDef) { v.Sources[0].Parameters = WidgetParameterBindings{{ID: ""}} }, "binding"},
		{"no category", func(v *ChartWidgetDef) { v.Chart.CategoryColumn = "" }, "categoryColumn"},
		{"no measure", func(v *ChartWidgetDef) { v.Chart.ValueColumn = "" }, "valueColumn"},
		{"unknown type", func(v *ChartWidgetDef) { v.Chart.Type = "pie" }, "type"},
		{"bad default view", func(v *ChartWidgetDef) { v.DefaultView = "table" }, "defaultView"},
		{"bar with period", func(v *ChartWidgetDef) { v.Chart.PeriodColumn = "month" }, "periodColumn"},
		{"race without period", func(v *ChartWidgetDef) { v.Chart.Type = "bar-race" }, "periodColumn"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := valid()
			tt.edit(&v)
			assert.ErrorContains(t, (BoardWidget{Name: "chart", Data: &v}).Validate(), tt.want)
		})
	}
	t.Run("race with period", func(t *testing.T) {
		v := valid()
		v.Chart.Type = "bar-race"
		v.Chart.PeriodColumn = "month"
		assert.NoError(t, (BoardWidget{Name: "chart", Data: &v}).Validate())
	})
	t.Run("typed nil", func(t *testing.T) {
		var v *ChartWidgetDef
		assert.ErrorContains(t, (BoardWidget{Name: "chart", Data: v}).Validate(), "null")
	})
	t.Run("runtime rows are not saved in chart data", func(t *testing.T) {
		assert.ErrorContains(t, (BoardWidget{Name: "chart", Data: map[string]any{
			"sources": []any{map[string]any{"id": "sqlite", "queryId": "q1"}},
			"chart":   map[string]any{"type": "bar", "categoryColumn": "ShipCountry", "valueColumn": "orders"},
			"rows":    []any{map[string]any{"ShipCountry": "USA", "orders": 1}},
		}}).Validate(), "unknown field")
	})
}

func TestTabsWidgetStableIDsPreserveLegacyTitles(t *testing.T) {
	legacy := TabsWidgetDef{Tabs: []TabWidget{{Title: "Same"}, {Title: "Same"}, {Title: ""}}}
	assert.NoError(t, legacy.Validate())
	assert.NoError(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: "one", Title: "Same"}, {ID: "two", Title: "Same"}}}).Validate())
	assert.ErrorContains(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: "same"}, {ID: "same"}}}).Validate(), "duplicate id")
	assert.ErrorContains(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: " padded "}}}).Validate(), "whitespace")
}

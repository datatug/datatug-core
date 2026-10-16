package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const savedChartBoardJSON = `{"id":"northwind","title":"Northwind","rows":[{"cards":[{"id":"countries","title":"Orders by country","widget":{"name":"tabs","data":{"tabs":[{"id":"comparison","title":"Compare sources","widget":{"name":"chart","data":{"sources":[{"id":"sqlite","queryId":"demodb/northwind-shipping-country-count-sqlite"},{"id":"postgresql","queryId":"demodb/northwind-shipping-country-count-postgresql"}],"chart":{"type":"bar","categoryColumn":"ShipCountry","valueColumn":"orders"},"defaultView":"chart"}}},{"id":"data","title":"Data","widget":{"name":"SQL","data":{"sql":{"queryId":"demodb/northwind-shipping-country-count-sqlite"}}}}]}}}]}]}`

const savedRaceBoardJSON = `{"id":"race","title":"Northwind race","rows":[{"cards":[{"id":"shipping-race","title":"Shipping race","widget":{"name":"chart","data":{"sources":[{"id":"orders","queryId":"demodb/northwind-shipping-country-race","transform":{"name":"orders-monthly-country-count","orderIdColumn":"OrderID","dateColumn":"OrderDate","countryColumn":"ShipCountry"}}],"chart":{"type":"bar-race","periodColumn":"period","categoryColumn":"category","valueColumn":"value","unit":"orders","grain":"month","onDuplicate":"reject","mode":"period","topN":10},"defaultView":"chart"}}}]}]}`

func TestBoardChartWireRoundTrip(t *testing.T) {
	for _, tc := range []struct{ name, fixture string }{{"comparison", savedChartBoardJSON}, {"race", savedRaceBoardJSON}} {
		t.Run(tc.name, func(t *testing.T) {
			var board Board
			require.NoError(t, json.Unmarshal([]byte(tc.fixture), &board))
			require.NoError(t, board.Validate())

			encoded, err := json.Marshal(board)
			require.NoError(t, err)
			var decoded Board
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.NoError(t, decoded.Validate())
			var before, after any
			require.NoError(t, json.Unmarshal([]byte(tc.fixture), &before))
			require.NoError(t, json.Unmarshal(encoded, &after))
			assert.Equal(t, before, after)
			assert.NotContains(t, string(encoded), "execution")
			assert.NotContains(t, string(encoded), "rowsData")
		})
	}
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
		{"padded source ID", func(v *ChartWidgetDef) { v.Sources[0].ID = " sqlite " }, "whitespace"},
		{"duplicate source ID", func(v *ChartWidgetDef) {
			v.Sources = append(v.Sources, ChartSourceBinding{ID: "sqlite", QueryID: "q2"})
		}, "duplicate id"},
		{"bad binding", func(v *ChartWidgetDef) { v.Sources[0].Parameters = WidgetParameterBindings{{ID: ""}} }, "binding"},
		{"no category", func(v *ChartWidgetDef) { v.Chart.CategoryColumn = "" }, "categoryColumn"},
		{"no measure", func(v *ChartWidgetDef) { v.Chart.ValueColumn = "" }, "valueColumn"},
		{"duplicate selectors", func(v *ChartWidgetDef) { v.Chart.ValueColumn = v.Chart.CategoryColumn }, "must differ"},
		{"unknown type", func(v *ChartWidgetDef) { v.Chart.Type = "pie" }, "type"},
		{"bad default view", func(v *ChartWidgetDef) { v.DefaultView = "table" }, "defaultView"},
		{"bar with period", func(v *ChartWidgetDef) { v.Chart.PeriodColumn = "month" }, "periodColumn"},
		{"bar with race options", func(v *ChartWidgetDef) { v.Chart.TopN = 10 }, "race options"},
		{"invalid widget title", func(v *ChartWidgetDef) { v.Title = strings.Repeat("x", MaxTitleLength+1) }, "title"},
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
		v.Chart.Unit = "orders"
		v.Chart.Grain = "month"
		v.Chart.OnDuplicate = "reject"
		v.Chart.Mode = "period"
		v.Chart.TopN = 10
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
	t.Run("unserializable chart data is rejected", func(t *testing.T) {
		assert.ErrorContains(t, (BoardWidget{Name: "chart", Data: make(chan int)}).Validate(), "unsupported type")
	})
}

func TestTabsWidgetStableIDsPreserveLegacyTitles(t *testing.T) {
	legacy := TabsWidgetDef{Tabs: []TabWidget{{Title: "Same"}, {Title: "Same"}, {Title: ""}}}
	assert.NoError(t, legacy.Validate())
	assert.NoError(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: "one", Title: "Same"}, {ID: "two", Title: "Same"}}}).Validate())
	assert.ErrorContains(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: "same"}, {ID: "same"}}}).Validate(), "duplicate id")
	assert.ErrorContains(t, (TabsWidgetDef{Tabs: []TabWidget{{ID: " padded "}}}).Validate(), "whitespace")
}

func TestChartRaceWireValidation(t *testing.T) {
	base := func() ChartWidgetDef {
		return ChartWidgetDef{
			Sources: []ChartSourceBinding{{ID: "orders", QueryID: "q1", Transform: &ChartSourceTransform{
				Name: "orders-monthly-country-count", OrderIDColumn: "OrderID", DateColumn: "OrderDate", CountryColumn: "ShipCountry",
			}}},
			Chart: ChartSpec{Type: "bar-race", PeriodColumn: "period", CategoryColumn: "category", ValueColumn: "value",
				Unit: "orders", Grain: "month", OnDuplicate: "reject", Mode: "period", TopN: 10},
		}
	}
	assert.NoError(t, base().Validate())
	cases := []struct {
		name string
		edit func(*ChartWidgetDef)
		want string
	}{
		{"unknown transform", func(v *ChartWidgetDef) { v.Sources[0].Transform.Name = "unknown" }, "name"},
		{"missing order column", func(v *ChartWidgetDef) { v.Sources[0].Transform.OrderIDColumn = "" }, "orderIdColumn"},
		{"duplicate input columns", func(v *ChartWidgetDef) { v.Sources[0].Transform.DateColumn = "OrderID" }, "must differ"},
		{"wrong transform output mapping", func(v *ChartWidgetDef) { v.Chart.PeriodColumn = "month" }, "mapped to period"},
		{"wrong transformed duplicate policy", func(v *ChartWidgetDef) { v.Chart.OnDuplicate = "sum" }, "rejecting duplicate"},
		{"missing unit", func(v *ChartWidgetDef) { v.Chart.Unit = "" }, "unit"},
		{"wrong grain", func(v *ChartWidgetDef) { v.Chart.Grain = "day" }, "grain"},
		{"missing duplicate policy", func(v *ChartWidgetDef) { v.Chart.OnDuplicate = "" }, "onDuplicate"},
		{"missing mode", func(v *ChartWidgetDef) { v.Chart.Mode = "" }, "mode"},
		{"top N zero", func(v *ChartWidgetDef) { v.Chart.TopN = 0 }, "topN"},
		{"top N too high", func(v *ChartWidgetDef) { v.Chart.TopN = 101 }, "topN"},
		{"duplicate selectors", func(v *ChartWidgetDef) { v.Chart.ValueColumn = "category" }, "must differ"},
		{"multiple race sources", func(v *ChartWidgetDef) { v.Sources = append(v.Sources, ChartSourceBinding{ID: "other", QueryID: "q2"}) }, "one source"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := base()
			tc.edit(&v)
			assert.ErrorContains(t, v.Validate(), tc.want)
		})
	}
}

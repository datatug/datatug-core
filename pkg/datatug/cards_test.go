package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sqlWidget(queryID string) *BoardWidget {
	return &BoardWidget{Name: string(QueryTypeSQL), Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: queryID}}}
}

func validCard() *Card {
	return &Card{
		ID:          "customer",
		Title:       "Customer",
		Description: "A customer",
		Display:     CardDisplayPanel,
		Parameters: []CardParameterDef{{
			ParameterDef: ParameterDef{ID: "customerId", Type: "string", IsRequired: true},
			Match: []CardMatchRule{
				{Entity: "Customer", Field: "CustomerID"},
				{Table: "Orders", Column: "CustomerID"},
				{Column: "CustomerID"},
			},
		}},
		Widget: sqlWidget("customer-by-id"),
	}
}

// A card the web saved with malformed match rules (admitted on purpose by the
// TypeScript validator, ignored when matching) is never refused by Go.
func TestCard_MalformedMatchRulesAreAdmitted(t *testing.T) {
	card := validCard()
	card.Parameters[0].Match = []CardMatchRule{{}, {Table: "Orders"}, {Entity: "E"}, {Column: "C", Field: "F"}}
	require.NoError(t, card.Validate())
	data, err := json.Marshal(card)
	require.NoError(t, err)
	var decoded Card
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, decoded.Validate())

	card.Parameters[0].Match = make([]CardMatchRule, MaxCardMatchRules)
	require.NoError(t, card.Validate())
	card.Parameters[0].Match = make([]CardMatchRule, MaxCardMatchRules+1)
	assert.ErrorContains(t, card.Validate(), "match")
}

func TestCardParameterDef_Validate(t *testing.T) {
	assert.NoError(t, CardParameterDef{ParameterDef: ParameterDef{ID: "a", Type: "string"}}.Validate())
	assert.Error(t, CardParameterDef{}.Validate())
	lookup := CardParameterDef{ParameterDef: ParameterDef{ID: "a", Type: "string", Lookup: &ParameterLookup{DB: "d", SQL: "select 1"}}}
	assert.ErrorContains(t, lookup.Validate(), "lookup")
	for _, kind := range []string{"record", "recordset"} {
		assert.ErrorContains(t, CardParameterDef{ParameterDef: ParameterDef{ID: "a", Type: kind, DefaultValue: "x"}}.Validate(), "defaultValue")
		assert.NoError(t, CardParameterDef{ParameterDef: ParameterDef{ID: "a", Type: kind}}.Validate())
	}
}

func TestBoardCardRef_Validate(t *testing.T) {
	assert.NoError(t, BoardCardRef{CardID: "customer", Parameters: WidgetParameterBindings{{ID: "customerId", BoardParameterID: "c"}}}.Validate())
	assert.Error(t, BoardCardRef{}.Validate())
	assert.Error(t, BoardCardRef{CardID: "../x"}.Validate())
	assert.Error(t, BoardCardRef{CardID: "a", Parameters: WidgetParameterBindings{{}}}.Validate())
}

func TestCard_Validate(t *testing.T) {
	assert.NoError(t, validCard().Validate())
	mutate := func(f func(*Card)) error {
		c := validCard()
		f(c)
		return c.Validate()
	}
	assert.Error(t, mutate(func(c *Card) { c.ID = "" }))
	for _, id := range []string{"a b", "a/b", "a.b", "..", strings.Repeat("x", 129)} {
		assert.Error(t, mutate(func(c *Card) { c.ID = id }), id)
	}
	assert.NoError(t, mutate(func(c *Card) { c.ID = strings.Repeat("x", 128) }))
	assert.Error(t, mutate(func(c *Card) { c.Title = " " }))
	assert.Error(t, mutate(func(c *Card) { c.Description = strings.Repeat("x", MaxCardDescriptionLength+1) }))
	assert.Error(t, mutate(func(c *Card) { c.Display = "popup" }))
	assert.NoError(t, mutate(func(c *Card) { c.Display = CardDisplayTooltip }))
	assert.NoError(t, mutate(func(c *Card) { c.Display = "" }))
	assert.Error(t, mutate(func(c *Card) { c.Parameters[0].Type = "" }))
	assert.ErrorContains(t, mutate(func(c *Card) { c.Parameters = append(c.Parameters, c.Parameters[0]) }), "duplicate parameter")
	assert.Error(t, mutate(func(c *Card) { c.Widget = nil }))
	assert.Error(t, mutate(func(c *Card) { c.Widget = &BoardWidget{Name: "nope"} }))
}

func TestCard_IDAndBrief(t *testing.T) {
	c := validCard()
	c.SetID("x")
	assert.Equal(t, "x", c.GetID())
	c.SetID("customer")
	brief := c.Brief()
	assert.Equal(t, "customer", brief.ID)
	assert.Equal(t, "Customer", brief.Title)
	assert.Equal(t, "A customer", brief.Description)
	assert.Equal(t, CardDisplayPanel, brief.Display)
	require.Len(t, brief.Parameters, 1)
	assert.Equal(t, CardBriefParameter{ID: "customerId", Type: "string", IsRequired: true, Match: c.Parameters[0].Match}, brief.Parameters[0])
	assert.Equal(t, []CardBrief{brief}, Cards{c}.Briefs())
	var alias = ProjectCardBrief(brief)
	assert.Equal(t, brief, alias)
}

func TestCards_Validate(t *testing.T) {
	assert.NoError(t, Cards{validCard()}.Validate())
	bad := validCard()
	bad.Title = ""
	assert.ErrorContains(t, Cards{validCard(), bad}.Validate(), "index 1")
}

func TestCard_JSON(t *testing.T) {
	data, err := json.Marshal(validCard())
	require.NoError(t, err)
	var decoded Card
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.NoError(t, decoded.Validate())
	again, err := json.Marshal(&decoded)
	require.NoError(t, err)
	assert.JSONEq(t, string(data), string(again))
	assert.Contains(t, string(data), `"match":[{"entity":"Customer","field":"CustomerID"},{"table":"Orders","column":"CustomerID"},{"column":"CustomerID"}]`)
	assert.Contains(t, string(data), `"isRequired":true`)

	for name, raw := range map[string]string{
		"unknown key":      `{"id":"a","title":"t","bogus":1}`,
		"ref in card":      `{"id":"a","title":"t","ref":{"cardId":"b"}}`,
		"unknown in param": `{"id":"a","title":"t","parameters":[{"id":"p","type":"string","x":1}]}`,
		"unknown in match": `{"id":"a","title":"t","parameters":[{"id":"p","type":"string","match":[{"col":"x"}]}]}`,
		"not an object":    `[]`,
	} {
		t.Run(name, func(t *testing.T) {
			var c Card
			assert.Error(t, json.Unmarshal([]byte(raw), &c))
		})
	}
}

func TestCard_JSON_KeepsExactNumbers(t *testing.T) {
	raw := `{"id":"c","title":"T","parameters":[{"id":"n","type":"integer","defaultValue":9007199254740993}],"widget":{"name":"fields","data":{"source":{"queryId":"q","parameters":[{"id":"n","value":9007199254740993}]}}}}`
	var card Card
	require.NoError(t, json.Unmarshal([]byte(raw), &card))
	require.NoError(t, card.Validate())
	again, err := json.Marshal(&card)
	require.NoError(t, err)
	assert.Contains(t, string(again), `"defaultValue":9007199254740993`)
	assert.Contains(t, string(again), `"value":9007199254740993`)
}

func TestCard_UnmarshalJSON_TrailingData(t *testing.T) {
	var c Card
	assert.ErrorContains(t, c.UnmarshalJSON([]byte(`{"id":"a"} {}`)), "unexpected data")
}

func TestBoardCard_RefValidate(t *testing.T) {
	ref := &BoardCardRef{CardID: "customer", Parameters: WidgetParameterBindings{{ID: "customerId", BoardParameterID: "c"}}}
	assert.NoError(t, BoardCard{ID: "c1", Ref: ref}.Validate())
	assert.ErrorContains(t, BoardCard{ID: "c1", Ref: ref, Widget: sqlWidget("q")}.Validate(), "mutually exclusive")
	assert.ErrorContains(t, BoardCard{ID: "c1"}.Validate(), "widget")
	assert.ErrorContains(t, BoardCard{ID: "c1", Ref: &BoardCardRef{}}.Validate(), "cardId")

	// A ref card inside a board is admitted, and survives a JSON round-trip.
	board := &Board{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b", Title: "B"}},
		Rows:        BoardRows{{Cards: BoardCards{{ID: "c1", Title: "Customer", Ref: ref}, {ID: "c2", Title: "W", Widget: sqlWidget("q")}}}},
	}
	require.NoError(t, board.Validate())
	data, err := json.Marshal(board)
	require.NoError(t, err)
	var decoded Board
	require.NoError(t, json.Unmarshal(data, &decoded))
	assert.Equal(t, ref, decoded.Rows[0].Cards[0].Ref)
	require.NoError(t, decoded.Validate())
}

func tabsWidget(tabs ...TabWidget) BoardWidget {
	return BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: tabs}}
}

func TestTabsWidgets_TextAndFields(t *testing.T) {
	text := BoardWidget{Name: "text", Data: &TextWidgetDef{WidgetBase: WidgetBase{Title: "Hello"}, Markdown: "Customer **{{customerId}}**"}}
	fields := BoardWidget{Name: "fields", Data: &FieldsWidgetDef{
		WidgetBase: WidgetBase{Title: "Profile"},
		Source:     FieldsWidgetSource{QueryID: "customer-by-id", Parameters: WidgetParameterBindings{{ID: "id", BoardParameterID: "customerId"}}},
		Fields:     []FieldsWidgetField{{Column: "CompanyName", Label: "Company"}},
	}}
	card := &Card{ID: "c", Title: "C", Widget: &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{
		{ID: "overview", Title: "Overview", Widgets: []BoardWidget{fields, text}},
		{Title: "Legacy", Widget: *sqlWidget("q")},
	}}}}
	require.NoError(t, card.Validate())

	// JSON round-trip: Data decodes to map[string]any and still validates.
	data, err := json.Marshal(card)
	require.NoError(t, err)
	var decoded Card
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, decoded.Validate())
	again, err := json.Marshal(&decoded)
	require.NoError(t, err)
	assert.JSONEq(t, string(data), string(again))
	for _, want := range []string{`"widgets":[`, `"name":"text"`, `"name":"fields"`, `"markdown":"Customer **{{customerId}}**"`, `"column":"CompanyName"`, `"label":"Company"`} {
		assert.Contains(t, string(data), want)
	}

	// Typed values (struct and pointer) are admitted too.
	typed := &BoardWidget{Name: "tabs", Data: TabsWidgetDef{Tabs: []TabWidget{{Widgets: []BoardWidget{
		{Name: "text", Data: TextWidgetDef{Markdown: "x"}},
		{Name: "fields", Data: FieldsWidgetDef{Source: FieldsWidgetSource{Parameter: "record"}}},
	}}}}}
	assert.NoError(t, typed.Validate())
}

func TestTextAndFieldsWidgets_Rejections(t *testing.T) {
	check := func(name, widgetJSON string) error {
		var w BoardWidget
		require.NoError(t, json.Unmarshal([]byte(widgetJSON), &w), name)
		return w.Validate()
	}
	assert.NoError(t, check("text", `{"name":"text","data":{"markdown":"hi"}}`))
	assert.Error(t, check("text unknown key", `{"name":"text","data":{"markdown":"hi","html":"<b>"}}`))
	assert.Error(t, check("text bad title", `{"name":"text","data":{"title":"`+strings.Repeat("x", MaxTitleLength+1)+`"}}`))
	assert.Error(t, check("text too long", `{"name":"text","data":{"markdown":"`+strings.Repeat("x", maxTextWidgetMarkdownLength+1)+`"}}`))
	assert.NoError(t, check("text null", `{"name":"text","data":null}`))
	assert.Error(t, check("text wrong type", `{"name":"text","data":{"markdown":5}}`))
	assert.NoError(t, check("fields query", `{"name":"fields","data":{"source":{"queryId":"q"}}}`))
	assert.NoError(t, check("fields param", `{"name":"fields","data":{"source":{"parameter":"rec"},"fields":[{"column":"A"}]}}`))
	assert.Error(t, check("fields unknown key", `{"name":"fields","data":{"source":{"queryId":"q"},"extra":1}}`))
	assert.Error(t, check("fields unknown source key", `{"name":"fields","data":{"source":{"queryId":"q","x":1}}}`))
	assert.Error(t, check("fields no source", `{"name":"fields","data":{"source":{}}}`))
	assert.Error(t, check("fields both sources", `{"name":"fields","data":{"source":{"queryId":"q","parameter":"p"}}}`))
	assert.Error(t, check("fields bad bindings", `{"name":"fields","data":{"source":{"queryId":"q","parameters":[{"id":""}]}}}`))
	assert.Error(t, check("fields param + bindings", `{"name":"fields","data":{"source":{"parameter":"p","parameters":[{"id":"a"}]}}}`))
	assert.Error(t, check("fields empty column", `{"name":"fields","data":{"source":{"queryId":"q"},"fields":[{"column":""}]}}`))
	assert.Error(t, check("fields bad title", `{"name":"fields","data":{"title":"`+strings.Repeat("x", MaxTitleLength+1)+`","source":{"queryId":"q"}}}`))
	assert.Error(t, check("fields null", `{"name":"fields","data":null}`))

	var nilText *TextWidgetDef
	assert.Error(t, (&BoardWidget{Name: "text", Data: nilText}).Validate())
	var nilFields *FieldsWidgetDef
	assert.Error(t, (&BoardWidget{Name: "fields", Data: nilFields}).Validate())

	// A bad widget in a tab's widgets list is reported with its position.
	bad := tabsWidget(TabWidget{Title: "T", Widgets: []BoardWidget{{Name: "nope"}}})
	assert.ErrorContains(t, bad.Validate(), "widget at index 0")
}

func TestPresentationLinks_VisitTabWidgets(t *testing.T) {
	// A presentation filter inside tab.widgets resolves against a chart in
	// another tab's widgets, and is reported when unresolved.
	filter := func(selection string) BoardWidget {
		return BoardWidget{Name: string(QueryTypeSQL), Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q", PresentationFilter: &PresentationFilter{SelectionID: selection, Column: "c"}}}}
	}
	mk := func(widgets ...BoardWidget) *Board {
		return &Board{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b", Title: "B"}},
			Rows: BoardRows{{Cards: BoardCards{
				{ID: "c1", Title: "C", Widget: &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{Title: "T", Widgets: widgets}}}}},
				{ID: "r", Title: "R", Ref: &BoardCardRef{CardID: "x"}},
			}}},
		}
	}
	assert.ErrorContains(t, mk(filter("missing")).validatePresentationLinks(), "unresolved selectionId")
	assert.NoError(t, mk(*sqlWidget("q")).validatePresentationLinks())
	// An undecodable widget inside tab.widgets is reported with its position.
	err := mk(BoardWidget{Name: "chart", Data: map[string]any{"unrecognized": true}}).validatePresentationLinks()
	assert.ErrorContains(t, err, "tab[0]/widget[0]")
}

func TestTabWidget_MarshalJSONOmitsAbsentLegacyWidget(t *testing.T) {
	typed := &TabsWidgetDef{Tabs: []TabWidget{
		{ID: "new", Title: "New", Widgets: []BoardWidget{{Name: "text", Data: &TextWidgetDef{Markdown: "x"}}}},
		{Title: "Placeholder"},
		{Title: "Legacy", Widget: *sqlWidget("q")},
	}}
	data, err := json.Marshal(typed)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"name":""`)
	assert.Contains(t, string(data), `"widgets":[{"name":"text"`)
	assert.Contains(t, string(data), `"widget":{"name":"SQL"`)
	var decoded TabsWidgetDef
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, decoded.Validate())
	assert.Equal(t, "", decoded.Tabs[0].Widget.Name)
	require.Len(t, decoded.Tabs[0].Widgets, 1)
	assert.Equal(t, "Legacy", decoded.Tabs[2].Title)
	assert.Equal(t, "SQL", decoded.Tabs[2].Widget.Name)
}

func TestCard_ValidatesPresentationLinksOfItsWidget(t *testing.T) {
	filter := func(selection string) *BoardWidget {
		return &BoardWidget{Name: string(QueryTypeSQL), Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q", PresentationFilter: &PresentationFilter{SelectionID: selection, Column: "c"}}}}
	}
	card := validCard()
	card.Widget = filter("missing")
	assert.ErrorContains(t, card.Validate(), "unresolved selectionId")
}

func TestSQLWidget_ParameterSource(t *testing.T) {
	check := func(wire string) error {
		var w BoardWidget
		require.NoError(t, json.Unmarshal([]byte(wire), &w))
		return w.Validate()
	}
	assert.NoError(t, check(`{"name":"SQL","data":{"title":"Rows","source":{"parameter":"rows"}}}`))
	assert.Error(t, check(`{"name":"SQL","data":{"source":{"parameter":""}}}`))
	assert.Error(t, check(`{"name":"SQL","data":{"source":{}}}`))
	assert.Error(t, check(`{"name":"SQL","data":{"source":{"parameter":"r","queryId":"q"}}}`), "strict decode")
	assert.Error(t, check(`{"name":"SQL","data":{"source":{"parameter":"r"},"extra":1}}`), "strict decode")
	assert.ErrorContains(t, check(`{"name":"SQL","data":{"source":{"parameter":"r"},"sql":{"queryId":"q"}}}`), "mutually exclusive")
	assert.ErrorContains(t, check(`{"name":"SQL","data":{"source":{"parameter":"r"},"sql":{}}}`), "mutually exclusive")

	// Typed values that carry both are refused too.
	both := &SQLWidgetDef{Source: &ParameterRowsSource{Parameter: "r"}, SQL: SQLWidgetSettings{QueryID: "q"}}
	assert.ErrorContains(t, both.Validate(), "mutually exclusive")
	both = &SQLWidgetDef{Source: &ParameterRowsSource{Parameter: "r"}, SQL: SQLWidgetSettings{Parameters: WidgetParameterBindings{{ID: "a"}}}}
	assert.Error(t, both.Validate())
	both = &SQLWidgetDef{Source: &ParameterRowsSource{Parameter: "r"}, SQL: SQLWidgetSettings{PresentationFilter: &PresentationFilter{SelectionID: "s", Column: "c"}}}
	assert.Error(t, both.Validate())

	// Marshal keeps "sql" out of a parameter-sourced widget and in a query one.
	sourced, err := json.Marshal(&SQLWidgetDef{Source: &ParameterRowsSource{Parameter: "r"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"source":{"parameter":"r"}}`, string(sourced))
	queried, err := json.Marshal(SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q"}})
	require.NoError(t, err)
	assert.JSONEq(t, `{"sql":{"queryId":"q"}}`, string(queried))

	// A card whose tab holds a parameter-sourced grid validates and round-trips,
	// and presentation-link validation copes with a widget that has no query.
	card := validCard()
	card.Parameters = append(card.Parameters, CardParameterDef{ParameterDef: ParameterDef{ID: "rows", Type: "recordset"}})
	card.Widget = &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{Title: "T", Widgets: []BoardWidget{
		{Name: string(QueryTypeSQL), Data: &SQLWidgetDef{Source: &ParameterRowsSource{Parameter: "rows"}}},
	}}}}}
	require.NoError(t, card.Validate())
	data, err := json.Marshal(card)
	require.NoError(t, err)
	var decoded Card
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NoError(t, decoded.Validate())
	assert.NotContains(t, string(data), `"sql"`)
	assert.Contains(t, string(data), `"source":{"parameter":"rows"}`)

	// Non-object data falls back to the permissive decoder and its error.
	assert.Error(t, (&BoardWidget{Name: string(QueryTypeSQL), Data: "text"}).Validate())
}

func TestTabWidget_WidgetAndWidgetsAreExclusive(t *testing.T) {
	both := BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{
		Title: "T", Widget: *sqlWidget("q"), Widgets: []BoardWidget{{Name: "text", Data: &TextWidgetDef{Markdown: "x"}}},
	}}}}
	assert.ErrorContains(t, both.Validate(), "either widget or widgets")
	var wire BoardWidget
	require.NoError(t, json.Unmarshal([]byte(`{"name":"tabs","data":{"tabs":[{"title":"T","widget":{"name":"text","data":{"markdown":"x"}},"widgets":[{"name":"text","data":{"markdown":"y"}}]}]}}`), &wire))
	assert.Error(t, wire.Validate())
}

func TestCard_RowsSourcesMustBeDeclaredRowParameters(t *testing.T) {
	build := func(types map[string]string, wire string) error {
		card := validCard()
		card.Parameters = nil
		for id, typ := range types {
			card.Parameters = append(card.Parameters, CardParameterDef{ParameterDef: ParameterDef{ID: id, Type: typ}})
		}
		var w BoardWidget
		require.NoError(t, json.Unmarshal([]byte(wire), &w))
		card.Widget = &w
		return card.Validate()
	}
	grid := `{"name":"SQL","data":{"source":{"parameter":"rows"}}}`
	fields := `{"name":"fields","data":{"source":{"parameter":"row"}}}`
	query := `{"name":"fields","data":{"source":{"queryId":"q"}}}`
	tabs := func(inner string) string {
		return `{"name":"tabs","data":{"tabs":[{"title":"A","widgets":[` + inner + `]},{"title":"B","widget":` + fields + `}]}}`
	}
	ok := map[string]string{"rows": "recordset", "row": "record"}
	assert.NoError(t, build(ok, grid))
	assert.NoError(t, build(ok, fields))
	assert.NoError(t, build(ok, query), "a query source names no parameter")
	assert.NoError(t, build(ok, tabs(grid)))
	assert.NoError(t, build(map[string]string{"rows": "recordset"}, grid), "a recordset feeds a grid")
	assert.NoError(t, build(map[string]string{"row": "recordset"}, fields), "a recordset also feeds fields")
	assert.ErrorContains(t, build(map[string]string{"row": "record"}, grid), `"rows"`)
	assert.ErrorContains(t, build(map[string]string{"rows": "string", "row": "record"}, grid), "declared record or recordset")
	assert.Error(t, build(map[string]string{"rows": "recordset"}, tabs(grid)), "the legacy tab widget is checked too")
	assert.Error(t, build(map[string]string{"rows": "recordset", "row": "record"}, tabs(`{"name":"SQL","data":{"source":{"parameter":"nope"}}}`)))
}

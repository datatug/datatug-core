package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBoards_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       Boards
		wantErr bool
	}{
		{
			name:    "empty",
			v:       Boards{},
			wantErr: false,
		},
		{
			name: "valid",
			v: Boards{
				&Board{
					ProjectItem: ProjectItem{
						ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_item",
			v: Boards{
				&Board{
					ProjectItem: ProjectItem{
						ProjItemBrief: ProjItemBrief{ID: ""},
					},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Boards.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoard_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       Board
		wantErr bool
	}{
		{
			name: "valid",
			v: Board{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_brief",
			v: Board{
				ProjectItem: ProjectItem{
					ProjItemBrief: ProjItemBrief{ID: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid_card_cols",
			v: Board{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"}},
				Rows: BoardRows{
					{Cards: BoardCards{{ID: "c1", Cols: -1}}},
				},
			},
			wantErr: true,
		},
		{
			name: "valid_parameters",
			v: Board{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"}},
				Parameters: Parameters{
					{ID: "p1", Type: "string"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_parameter_missing_id",
			v: Board{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"}},
				Parameters: Parameters{
					{ID: "", Type: "string"},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid_parameter_missing_type",
			v: Board{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "p1", Title: "Board 1"}},
				Parameters: Parameters{
					{ID: "p1", Type: ""},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Board.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestBoard_JSONRoundTrip proves that Parameters and RequiredParams survive a
// board.json marshal/unmarshal round trip instead of being silently dropped.
func TestBoard_JSONRoundTrip(t *testing.T) {
	original := Board{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"}},
		Parameters: Parameters{
			{ID: "p1", Type: "string", Title: "Param 1"},
			{ID: "p2", Type: "integer", IsRequired: true},
		},
		RequiredParams: [][]string{
			{"p1"},
			{"p2", "p1"},
		},
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var roundTripped Board
	assert.NoError(t, json.Unmarshal(data, &roundTripped))

	assert.Equal(t, original.Parameters, roundTripped.Parameters)
	assert.Equal(t, original.RequiredParams, roundTripped.RequiredParams)
}

func TestBoardWidget_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       BoardWidget
		wantErr bool
	}{
		{
			name:    "empty_name",
			v:       BoardWidget{Name: ""},
			wantErr: true,
		},
		{
			name:    "unknown_name",
			v:       BoardWidget{Name: "unknown"},
			wantErr: true,
		},
		{
			name: "sql_valid_pointer",
			v: BoardWidget{
				Name: "SQL",
				Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}},
			},
			wantErr: false,
		},
		{
			name: "sql_valid_map",
			v: BoardWidget{
				Name: "SQL",
				Data: map[string]interface{}{
					"sql": map[string]interface{}{"queryId": "q1"},
				},
			},
			wantErr: false,
		},
		{
			name: "http_valid_pointer",
			v: BoardWidget{
				Name: "HTTP",
				Data: &HTTPWidgetDef{Request: HTTPRequest{URL: "http://example.com", Method: "GET"}},
			},
			wantErr: false,
		},
		{
			name: "http_valid_map",
			v: BoardWidget{
				Name: "HTTP",
				Data: map[string]interface{}{
					"request": map[string]interface{}{"url": "http://example.com", "method": "GET"},
				},
			},
			wantErr: false,
		},
		{
			name: "tabs_valid_pointer",
			v: BoardWidget{
				Name: "tabs",
				Data: &TabsWidgetDef{
					Tabs: []TabWidget{
						{Title: "Tab 1", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}}}},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "tabs_valid_map",
			v: BoardWidget{
				Name: "tabs",
				Data: map[string]interface{}{
					"tabs": []interface{}{
						map[string]interface{}{
							"title":  "Tab 1",
							"widget": map[string]interface{}{"name": "SQL", "data": map[string]interface{}{"sql": map[string]interface{}{"queryId": "q1"}}},
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "sql_invalid_data",
			v: BoardWidget{
				Name: "SQL",
				Data: "invalid",
			},
			wantErr: true,
		},
		{
			name: "sql_marshal_error",
			v: BoardWidget{
				Name: "SQL",
				Data: make(chan int),
			},
			wantErr: true,
		},
		{
			name: "http_marshal_error",
			v: BoardWidget{
				Name: "HTTP",
				Data: make(chan int),
			},
			wantErr: true,
		},
		{
			name: "tabs_marshal_error",
			v: BoardWidget{
				Name: "tabs",
				Data: make(chan int),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("BoardWidget.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWidgetBase_Validate(t *testing.T) {
	tests := []struct {
		name            string
		v               WidgetBase
		isTitleRequired bool
		wantErr         bool
	}{
		{
			name:            "valid_no_title_not_required",
			v:               WidgetBase{},
			isTitleRequired: false,
			wantErr:         false,
		},
		{
			name:            "invalid_no_title_required",
			v:               WidgetBase{},
			isTitleRequired: true,
			wantErr:         true,
		},
		{
			name: "invalid_parameters",
			v: WidgetBase{
				Parameters: Parameters{{ID: ""}},
			},
			isTitleRequired: false,
			wantErr:         true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(tt.isTitleRequired); (err != nil) != tt.wantErr {
				t.Errorf("WidgetBase.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPHeaderItem_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       HTTPHeaderItem
		wantErr bool
	}{
		{
			name:    "valid",
			v:       HTTPHeaderItem{Name: "Content-Type", Value: "application/json"},
			wantErr: false,
		},
		{
			name:    "missing_name",
			v:       HTTPHeaderItem{Value: "application/json"},
			wantErr: true,
		},
		{
			name:    "missing_value",
			v:       HTTPHeaderItem{Name: "Content-Type"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPHeaderItem.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPHeaders_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       HTTPHeaders
		wantErr bool
	}{
		{
			name:    "valid",
			v:       HTTPHeaders{{Name: "Content-Type", Value: "application/json"}},
			wantErr: false,
		},
		{
			name:    "invalid_item",
			v:       HTTPHeaders{{Name: ""}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPHeaders.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestHTTPRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       HTTPRequest
		wantErr bool
	}{
		{
			name:    "valid_get",
			v:       HTTPRequest{URL: "http://example.com", Method: "GET"},
			wantErr: false,
		},
		{
			name:    "missing_url",
			v:       HTTPRequest{Method: "GET"},
			wantErr: true,
		},
		{
			name:    "negative_timeout",
			v:       HTTPRequest{URL: "http://example.com", Method: "GET", TimeoutThresholdMs: -1},
			wantErr: true,
		},
		{
			name:    "missing_method",
			v:       HTTPRequest{URL: "http://example.com"},
			wantErr: true,
		},
		{
			name:    "get_with_content",
			v:       HTTPRequest{URL: "http://example.com", Method: "GET", Content: "some content"},
			wantErr: true,
		},
		{
			name:    "valid_post",
			v:       HTTPRequest{URL: "http://example.com", Method: "POST", Content: "some content"},
			wantErr: false,
		},
		{
			name:    "valid_put",
			v:       HTTPRequest{URL: "http://example.com", Method: "PUT", Content: "some content"},
			wantErr: false,
		},
		{
			name:    "valid_head",
			v:       HTTPRequest{URL: "http://example.com", Method: "HEAD"},
			wantErr: false,
		},
		{
			name:    "valid_options",
			v:       HTTPRequest{URL: "http://example.com", Method: "OPTIONS"},
			wantErr: false,
		},
		{
			name:    "valid_delete",
			v:       HTTPRequest{URL: "http://example.com", Method: "DELETE"},
			wantErr: false,
		},
		{
			name:    "valid_patch",
			v:       HTTPRequest{URL: "http://example.com", Method: "PATCH"},
			wantErr: false,
		},
		{
			name:    "valid_trace",
			v:       HTTPRequest{URL: "http://example.com", Method: "TRACE"},
			wantErr: false,
		},
		{
			name:    "valid_connect",
			v:       HTTPRequest{URL: "http://example.com", Method: "CONNECT"},
			wantErr: false,
		},
		{
			name:    "unknown_method",
			v:       HTTPRequest{URL: "http://example.com", Method: "UNKNOWN"},
			wantErr: true,
		},
		{
			name:    "invalid_headers",
			v:       HTTPRequest{URL: "http://example.com", Method: "GET", Headers: HTTPHeaders{{Name: ""}}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPRequest.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProjBoardBrief_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       ProjBoardBrief
		wantErr bool
	}{
		{
			name: "valid",
			v: ProjBoardBrief{
				ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
			},
			wantErr: false,
		},
		{
			name: "invalid_title",
			v: ProjBoardBrief{
				ProjItemBrief: ProjItemBrief{ID: "b1", Title: ""},
			},
			wantErr: true,
		},
		{
			name: "invalid_parameter",
			v: ProjBoardBrief{
				ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
				Parameters: Parameters{
					{ID: ""},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("ProjBoardBrief.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoardRows_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       BoardRows
		wantErr bool
	}{
		{
			name:    "empty",
			v:       BoardRows{},
			wantErr: false,
		},
		{
			name: "valid",
			v: BoardRows{
				{Cards: BoardCards{{ID: "c1", Widget: &BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}}}}}},
			},
			wantErr: false,
		},
		{
			name: "invalid_row",
			v: BoardRows{
				{Cards: BoardCards{{ID: ""}}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("BoardRows.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoardCard_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       BoardCard
		wantErr bool
	}{
		{
			name: "valid",
			v: BoardCard{
				ID: "c1",
				Widget: &BoardWidget{
					Name: "SQL",
					Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}},
				},
			},
			wantErr: false,
		},
		{
			name:    "missing_id",
			v:       BoardCard{ID: ""},
			wantErr: true,
		},
		{
			name:    "negative_cols",
			v:       BoardCard{ID: "c1", Cols: -1},
			wantErr: true,
		},
		{
			name:    "missing_widget",
			v:       BoardCard{ID: "c1"},
			wantErr: true,
		},
		{
			name: "invalid_widget",
			v: BoardCard{
				ID:     "c1",
				Widget: &BoardWidget{Name: ""},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("BoardCard.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBoardMissingCardWidgetReturnsValidationError(t *testing.T) {
	card := BoardCard{ID: "empty-card"}
	assert.ErrorContains(t, card.Validate(), "widget")

	board := Board{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "board", Title: "Board"}},
		Rows:        BoardRows{{Cards: BoardCards{card}}},
	}
	assert.ErrorContains(t, board.Validate(), "widget")

	for _, wire := range []string{
		`{"id":"board","title":"Board","rows":[{"cards":[{"id":"empty-card"}]}]}`,
		`{"id":"board","title":"Board","rows":[{"cards":[{"id":"empty-card","widget":null}]}]}`,
	} {
		var raw Board
		assert.NoError(t, json.Unmarshal([]byte(wire), &raw))
		assert.ErrorContains(t, raw.Validate(), "widget")
	}

	// Empty tab children remain placeholders inside a real card widget.
	const placeholder = `{"id":"board","title":"Board","rows":[{"cards":[{"id":"tabs-card","widget":{"name":"tabs","data":{"tabs":[{"title":"Empty"}]}}}]}]}`
	var raw Board
	assert.NoError(t, json.Unmarshal([]byte(placeholder), &raw))
	assert.NoError(t, raw.Validate())
}

func TestTabsWidgetDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := TabsWidgetDef{
			WidgetBase: WidgetBase{Title: "Tabs"},
			Tabs: []TabWidget{
				{Title: "Tab 1", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}}}},
			},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_base", func(t *testing.T) {
		v := TabsWidgetDef{
			WidgetBase: WidgetBase{Title: string(make([]byte, MaxTitleLength+1))},
		}
		assert.Error(t, v.Validate())
	})
}

func TestTabsWidgetDef_ValidateChildren(t *testing.T) {
	validSQL := func() BoardWidget {
		return BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{QueryID: "q1"}}}
	}
	validHTTP := func() BoardWidget {
		return BoardWidget{Name: "HTTP", Data: &HTTPWidgetDef{Request: HTTPRequest{Method: "GET", URL: "https://example.com"}}}
	}
	tests := []struct {
		name    string
		tabs    []TabWidget
		wantErr string
	}{
		{name: "empty tabs remain valid"},
		{name: "SQL and HTTP", tabs: []TabWidget{{Title: "Query", Widget: validSQL()}, {Title: "Feed", Widget: validHTTP()}}},
		{name: "nested tabs", tabs: []TabWidget{{Title: "Outer", Widget: BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{Title: "Inner", Widget: validSQL()}}}}}}},
		{name: "legacy missing title", tabs: []TabWidget{{Widget: validSQL()}}},
		{name: "legacy whitespace title", tabs: []TabWidget{{Title: "  ", Widget: validSQL()}}},
		{name: "legacy padded title", tabs: []TabWidget{{Title: " Query ", Widget: validSQL()}}},
		{name: "legacy duplicate title", tabs: []TabWidget{{Title: "Query", Widget: validSQL()}, {Title: "Query", Widget: validHTTP()}}},
		{name: "placeholder without child widget", tabs: []TabWidget{{Title: "Query"}}},
		{name: "malformed child without name", tabs: []TabWidget{{Title: "Query", Widget: BoardWidget{Data: map[string]interface{}{"sql": map[string]interface{}{"queryId": "q1"}}}}}, wantErr: "name"},
		{name: "invalid SQL child", tabs: []TabWidget{{Title: "Query", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{}}}}, wantErr: "queryId"},
		{name: "invalid HTTP child", tabs: []TabWidget{{Title: "Feed", Widget: BoardWidget{Name: "HTTP", Data: &HTTPWidgetDef{Request: HTTPRequest{Method: "GET"}}}}}, wantErr: "url"},
		{
			name: "invalid grandchild",
			tabs: []TabWidget{{
				Title: "Outer",
				Widget: BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{
					Title:  "Inner",
					Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{}},
				}}}},
			}},
			wantErr: "queryId",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := (TabsWidgetDef{Tabs: tt.tabs}).Validate()
			if tt.wantErr == "" {
				assert.NoError(t, err)
			} else {
				assert.ErrorContains(t, err, tt.wantErr)
			}
		})
	}
}

func TestTabsWidgetDef_ValidateCycle(t *testing.T) {
	tabs := &TabsWidgetDef{}
	tabs.Tabs = []TabWidget{{Title: "Loop", Widget: BoardWidget{Name: "tabs", Data: tabs}}}
	assert.ErrorContains(t, tabs.Validate(), "nesting exceeds maximum depth")
}

func TestBoardWidget_ValidateTabsNilData(t *testing.T) {
	assert.NoError(t, (BoardWidget{Name: "tabs"}).Validate())
	var nilTabs *TabsWidgetDef
	assert.NoError(t, (BoardWidget{Name: "tabs", Data: nilTabs}).Validate())
}

func TestBoardWidget_ValidateTypedNilQueryData(t *testing.T) {
	var nilSQL *SQLWidgetDef
	assert.ErrorContains(t, (BoardWidget{Name: "SQL", Data: nilSQL}).Validate(), "SQL widget definition is null")
	var nilHTTP *HTTPWidgetDef
	assert.ErrorContains(t, (BoardWidget{Name: "HTTP", Data: nilHTTP}).Validate(), "HTTP widget definition is null")
}

func TestBoard_ValidateLegacyTabsJSONRoundTrip(t *testing.T) {
	const legacy = `{"id":"b1","title":"Board","rows":[{"cards":[{"id":"c1","widget":{"name":"tabs","data":{"tabs":[{"title":""},{"title":"Same","widget":{}},{"title":"Same","widget":{"name":"SQL","data":{"sql":{"queryId":"q1"}}}}]}}}]}]}`
	var board Board
	assert.NoError(t, json.Unmarshal([]byte(legacy), &board))
	assert.NoError(t, board.Validate())
	data, err := json.Marshal(board)
	assert.NoError(t, err)
	var roundTripped Board
	assert.NoError(t, json.Unmarshal(data, &roundTripped))
	assert.NoError(t, roundTripped.Validate())
}

func TestBoard_ValidateNestedTabFromJSON(t *testing.T) {
	original := Board{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board"}},
		Rows: BoardRows{{Cards: BoardCards{{
			ID: "c1",
			Widget: &BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{
				Title: "Outer",
				Widget: BoardWidget{Name: "tabs", Data: &TabsWidgetDef{Tabs: []TabWidget{{
					Title: "Query", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{}},
				}}}},
			}}}},
		}}}},
	}
	data, err := json.Marshal(original)
	assert.NoError(t, err)
	var board Board
	assert.NoError(t, json.Unmarshal(data, &board))
	assert.ErrorContains(t, board.Validate(), "queryId")
}

func TestSQLWidgetDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := SQLWidgetDef{
			SQL: SQLWidgetSettings{QueryID: "q1"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_base", func(t *testing.T) {
		v := SQLWidgetDef{
			WidgetBase: WidgetBase{Parameters: Parameters{{ID: ""}}},
			SQL:        SQLWidgetSettings{QueryID: "q1"},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_sql", func(t *testing.T) {
		v := SQLWidgetDef{
			SQL: SQLWidgetSettings{QueryID: ""},
		}
		assert.Error(t, v.Validate())
	})
}

func TestSQLWidgetSettings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       SQLWidgetSettings
		wantErr bool
	}{
		{
			name:    "valid_query_id_only",
			v:       SQLWidgetSettings{QueryID: "q1"},
			wantErr: false,
		},
		{
			name: "valid_query_id_and_value_binding",
			v: SQLWidgetSettings{
				QueryID:    "q1",
				Parameters: WidgetParameterBindings{{ID: "p1", Value: "2026"}},
			},
			wantErr: false,
		},
		{
			name: "valid_query_id_and_board_parameter_binding",
			v: SQLWidgetSettings{
				QueryID:    "q1",
				Parameters: WidgetParameterBindings{{ID: "p1", BoardParameterID: "year"}},
			},
			wantErr: false,
		},
		{
			name: "valid_binding_with_neither_value_nor_board_parameter_id",
			v: SQLWidgetSettings{
				QueryID:    "q1",
				Parameters: WidgetParameterBindings{{ID: "p1"}},
			},
			wantErr: false,
		},
		{
			name:    "missing_query_id",
			v:       SQLWidgetSettings{},
			wantErr: true,
		},
		{
			name: "invalid_binding_missing_id",
			v: SQLWidgetSettings{
				QueryID:    "q1",
				Parameters: WidgetParameterBindings{{ID: "", Value: "2026"}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("SQLWidgetSettings.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWidgetParameterBindings_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       WidgetParameterBindings
		wantErr bool
	}{
		{
			name:    "empty",
			v:       WidgetParameterBindings{},
			wantErr: false,
		},
		{
			name:    "valid",
			v:       WidgetParameterBindings{{ID: "p1", Value: "2026"}, {ID: "p2", BoardParameterID: "region"}},
			wantErr: false,
		},
		{
			name:    "invalid_item",
			v:       WidgetParameterBindings{{ID: ""}},
			wantErr: true,
		},
		{
			name:    "duplicate_binding_ids",
			v:       WidgetParameterBindings{{ID: "p1", Value: "a"}, {ID: "p1", Value: "b"}},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("WidgetParameterBindings.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWidgetParameterBinding_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       WidgetParameterBinding
		wantErr bool
	}{
		{
			name:    "valid_value_only",
			v:       WidgetParameterBinding{ID: "p1", Value: "2026"},
			wantErr: false,
		},
		{
			name:    "valid_board_parameter_id_only",
			v:       WidgetParameterBinding{ID: "p1", BoardParameterID: "year"},
			wantErr: false,
		},
		{
			name:    "valid_neither_set_uses_query_parameter_default",
			v:       WidgetParameterBinding{ID: "p1"},
			wantErr: false,
		},
		{
			name:    "missing_id",
			v:       WidgetParameterBinding{Value: "2026"},
			wantErr: true,
		},
		{
			name:    "both_value_and_board_parameter_id",
			v:       WidgetParameterBinding{ID: "p1", Value: "2026", BoardParameterID: "year"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("WidgetParameterBinding.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestSQLWidgetDef_JSONRoundTrip proves that a SQLWidgetDef with a queryId and
// both binding kinds (constant value and board-parameter reference) survives
// a marshal/unmarshal round trip, instead of the removed inline Query field.
func TestSQLWidgetDef_JSONRoundTrip(t *testing.T) {
	original := SQLWidgetDef{
		WidgetBase: WidgetBase{Title: "Revenue by region"},
		SQL: SQLWidgetSettings{
			QueryID: "q1",
			Parameters: WidgetParameterBindings{
				{ID: "p1", Value: "2026"},
				{ID: "p2", BoardParameterID: "region"},
			},
		},
	}

	data, err := json.Marshal(original)
	assert.NoError(t, err)

	var roundTripped SQLWidgetDef
	assert.NoError(t, json.Unmarshal(data, &roundTripped))

	assert.Equal(t, original, roundTripped)
}

func TestHTTPWidgetDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := HTTPWidgetDef{
			Request: HTTPRequest{URL: "http://example.com", Method: "GET"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_base", func(t *testing.T) {
		v := HTTPWidgetDef{
			WidgetBase: WidgetBase{Parameters: Parameters{{ID: ""}}},
			Request:    HTTPRequest{URL: "http://example.com", Method: "GET"},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_request", func(t *testing.T) {
		v := HTTPWidgetDef{
			Request: HTTPRequest{URL: "", Method: "GET"},
		}
		assert.Error(t, v.Validate())
	})
}

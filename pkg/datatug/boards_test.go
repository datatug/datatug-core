package datatug

import (
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Board.ValidateWithOptions() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
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
				Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}},
			},
			wantErr: false,
		},
		{
			name: "sql_valid_map",
			v: BoardWidget{
				Name: "SQL",
				Data: map[string]interface{}{
					"sql": map[string]interface{}{"query": "SELECT 1"},
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
						{Title: "Tab 1", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}}}},
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
							"widget": map[string]interface{}{"name": "SQL", "data": map[string]interface{}{"sql": map[string]interface{}{"query": "SELECT 1"}}},
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
				{Cards: BoardCards{{ID: "c1", Widget: &BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}}}}}},
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
					Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}},
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

func TestTabsWidgetDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := TabsWidgetDef{
			WidgetBase: WidgetBase{Title: "Tabs"},
			Tabs: []TabWidget{
				{Title: "Tab 1", Widget: BoardWidget{Name: "SQL", Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}}}},
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

func TestSQLWidgetDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := SQLWidgetDef{
			SQL: SQLWidgetSettings{Query: "SELECT 1"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_base", func(t *testing.T) {
		v := SQLWidgetDef{
			WidgetBase: WidgetBase{Parameters: Parameters{{ID: ""}}},
			SQL:        SQLWidgetSettings{Query: "SELECT 1"},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_sql", func(t *testing.T) {
		v := SQLWidgetDef{
			SQL: SQLWidgetSettings{Query: ""},
		}
		assert.Error(t, v.Validate())
	})
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

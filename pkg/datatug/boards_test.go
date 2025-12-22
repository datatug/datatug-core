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
					ProjBoardBrief: ProjBoardBrief{
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
					ProjBoardBrief: ProjBoardBrief{
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
				t.Errorf("Boards.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
				ProjBoardBrief: ProjBoardBrief{
					ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid_brief",
			v: Board{
				ProjBoardBrief: ProjBoardBrief{
					ProjItemBrief: ProjItemBrief{ID: ""},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid_rows",
			v: Board{
				ProjBoardBrief: ProjBoardBrief{
					ProjItemBrief: ProjItemBrief{ID: "b1", Title: "Board 1"},
				},
				Rows: BoardRows{
					{Cards: BoardCards{{ID: ""}}},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Board.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
				t.Errorf("ProjBoardBrief.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
				t.Errorf("BoardRows.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
				t.Errorf("BoardCard.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
			name: "valid_sql",
			v: BoardWidget{
				Name: "SQL",
				Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: "SELECT 1"}},
			},
			wantErr: false,
		},
		{
			name: "valid_sql_map",
			v: BoardWidget{
				Name: "SQL",
				Data: map[string]interface{}{"sql": map[string]interface{}{"query": "SELECT 1"}},
			},
			wantErr: false,
		},
		{
			name: "valid_http",
			v: BoardWidget{
				Name: "HTTP",
				Data: &HTTPWidgetDef{Request: HTTPRequest{Method: "GET", URL: "http://example.com"}},
			},
			wantErr: false,
		},
		{
			name: "valid_tabs",
			v: BoardWidget{
				Name: "tabs",
				Data: &TabsWidgetDef{},
			},
			wantErr: false,
		},
		{
			name:    "missing_name",
			v:       BoardWidget{Name: ""},
			wantErr: true,
		},
		{
			name:    "unknown_name",
			v:       BoardWidget{Name: "unknown"},
			wantErr: true,
		},
		{
			name: "invalid_sql_data",
			v: BoardWidget{
				Name: "SQL",
				Data: &SQLWidgetDef{SQL: SQLWidgetSettings{Query: ""}},
			},
			wantErr: true,
		},
		{
			name: "invalid_data_type",
			v: BoardWidget{
				Name: "SQL",
				Data: "invalid",
			},
			wantErr: true,
		},
		{
			name: "invalid_json_data",
			v: BoardWidget{
				Name: "SQL",
				Data: map[string]interface{}{"sql": "invalid"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("BoardWidget.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWidgetBase_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, WidgetBase{Title: "T"}.Validate(true))
	})
	t.Run("too_long_title", func(t *testing.T) {
		title := ""
		for i := 0; i < 101; i++ {
			title += "a"
		}
		assert.Error(t, WidgetBase{Title: title}.Validate(false))
	})
}

func TestTabsWidgetDef_Validate(t *testing.T) {
	assert.NoError(t, (&TabsWidgetDef{}).Validate())
}

func TestHTTPRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		v       HTTPRequest
		wantErr bool
	}{
		{
			name:    "valid_get",
			v:       HTTPRequest{Method: "GET", URL: "http://example.com"},
			wantErr: false,
		},
		{
			name:    "missing_url",
			v:       HTTPRequest{Method: "GET", URL: ""},
			wantErr: true,
		},
		{
			name:    "negative_timeout",
			v:       HTTPRequest{Method: "GET", URL: "http://example.com", TimeoutThresholdMs: -1},
			wantErr: true,
		},
		{
			name:    "missing_method",
			v:       HTTPRequest{Method: "", URL: "http://example.com"},
			wantErr: true,
		},
		{
			name:    "get_with_content",
			v:       HTTPRequest{Method: "GET", URL: "http://example.com", Content: "some content"},
			wantErr: true,
		},
		{
			name:    "unknown_method",
			v:       HTTPRequest{Method: "UNKNOWN", URL: "http://example.com"},
			wantErr: true,
		},
		{
			name: "invalid_headers",
			v: HTTPRequest{
				Method: "GET",
				URL:    "http://example.com",
				Headers: HTTPHeaders{
					{Name: ""},
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPRequest.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
			name: "valid",
			v: HTTPHeaders{
				{Name: "Content-Type", Value: "application/json"},
			},
			wantErr: false,
		},
		{
			name: "invalid_item",
			v: HTTPHeaders{
				{Name: "", Value: "application/json"},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPHeaders.Validate() error = %v, wantErr %v", err, tt.wantErr)
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
			v:       HTTPHeaderItem{Name: "N", Value: "V"},
			wantErr: false,
		},
		{
			name:    "missing_name",
			v:       HTTPHeaderItem{Name: "", Value: "V"},
			wantErr: true,
		},
		{
			name:    "missing_value",
			v:       HTTPHeaderItem{Name: "N", Value: ""},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.v.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("HTTPHeaderItem.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

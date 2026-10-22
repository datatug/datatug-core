package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/strongo/validation"
)

// Boards is a slice of *Board
type Boards ProjectItems[*Board]

// Validate returns error if failed
func (v Boards) Validate() error {
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid board at index %v: %w", i, err)
		}
	}
	return nil
}

// Board is holding all details about board
type Board struct {
	ProjectItem

	// Parameters mirrors ProjBoardBrief.Parameters so the brief and the full board stay in step.
	Parameters Parameters `json:"parameters,omitempty" firestore:"parameters,omitempty"`

	// RequiredParams mirrors ProjBoardBrief.RequiredParams so the brief and the full board stay in step.
	RequiredParams [][]string `json:"requiredParams,omitempty" firestore:"requiredParams,omitempty"`

	Rows BoardRows `json:"rows,omitempty" firestore:"rows,omitempty"`
}

// Validate returns error if failed
func (v *Board) Validate() error {
	if err := v.ValidateWithOptions(true); err != nil {
		return err
	}
	for i, p := range v.Parameters {
		if err := p.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("parameters", fmt.Sprintf("invalid parameter #%v: %v", i+1, err))
		}
	}
	if err := v.Rows.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("rows", err.Error())
	}
	if err := v.validatePresentationLinks(); err != nil {
		return validation.NewErrBadRecordFieldValue("rows", err.Error())
	}
	return nil
}

type presentationLink struct {
	path        string
	selectionID string
}

// Widget.Validate checks a widget's own shape. Only the complete Board can
// resolve a presentation filter against a chart in another card or tab.
func (v *Board) validatePresentationLinks() error {
	var roots []presentationRoot
	for rowIndex, row := range v.Rows {
		for cardIndex, card := range row.Cards {
			if card.Widget == nil {
				continue // a ref card has no inline widget
			}
			roots = append(roots, presentationRoot{widget: *card.Widget, path: fmt.Sprintf("row[%d]/card[%d]", rowIndex, cardIndex)})
		}
	}
	return validatePresentationRoots(roots)
}

// presentationRoot is the top widget of one card and the path that names it.
type presentationRoot struct {
	widget BoardWidget
	path   string
}

// validatePresentationRoots resolves every presentation filter against the
// charts produced anywhere in the given roots: all cards of a board, or the
// one widget of a project card.
func validatePresentationRoots(roots []presentationRoot) error {
	producers := make(map[string]string)
	var filters []presentationLink
	var visit func(BoardWidget, int, string) error
	visit = func(widget BoardWidget, depth int, path string) error {
		if depth > maxWidgetValidationDepth {
			return fmt.Errorf("%s: widget nesting exceeds maximum depth", path)
		}
		switch widget.Name {
		case "chart":
			var chart *ChartWidgetDef
			switch typed := widget.Data.(type) {
			case *ChartWidgetDef:
				chart = typed
			case ChartWidgetDef:
				chart = &typed
			default:
				decoded, err := newStrictWidgetDef(&ChartWidgetDef{}, widget.Data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				chart = decoded.(*ChartWidgetDef)
			}
			if chart != nil && chart.Chart.SelectionID != "" {
				id := chart.Chart.SelectionID
				if previous, duplicate := producers[id]; duplicate {
					return fmt.Errorf("%s: duplicate selectionId %q also produced at %s", path, id, previous)
				}
				producers[id] = path
			}
		case string(QueryTypeSQL):
			var sql *SQLWidgetDef
			switch typed := widget.Data.(type) {
			case *SQLWidgetDef:
				sql = typed
			case SQLWidgetDef:
				sql = &typed
			default:
				decoded, err := decodeSQLWidgetDef(widget.Data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				sql = decoded.(*SQLWidgetDef)
			}
			if sql != nil && sql.SQL.PresentationFilter != nil {
				filters = append(filters, presentationLink{path: path, selectionID: sql.SQL.PresentationFilter.SelectionID})
			}
		case "tabs":
			var tabs *TabsWidgetDef
			switch typed := widget.Data.(type) {
			case *TabsWidgetDef:
				tabs = typed
			case TabsWidgetDef:
				tabs = &typed
			default:
				decoded, err := newWidgetDef(&TabsWidgetDef{}, widget.Data)
				if err != nil {
					return fmt.Errorf("%s: %w", path, err)
				}
				tabs = decoded.(*TabsWidgetDef)
			}
			if tabs == nil {
				return nil
			}
			for i, tab := range tabs.Tabs {
				for j, widget := range tab.Widgets {
					if err := visit(widget, depth+1, fmt.Sprintf("%s/tab[%d]/widget[%d]", path, i, j)); err != nil {
						return err
					}
				}
				if tab.Widget.Name == "" && tab.Widget.Data == nil {
					continue
				}
				if err := visit(tab.Widget, depth+1, fmt.Sprintf("%s/tab[%d]", path, i)); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, root := range roots {
		if err := visit(root.widget, 0, root.path); err != nil {
			return err
		}
	}
	for _, filter := range filters {
		if _, exists := producers[filter.selectionID]; !exists {
			return fmt.Errorf("%s: unresolved selectionId %q", filter.path, filter.selectionID)
		}
	}
	return nil
}

// ProjBoardBrief defines brief information of Board
type ProjBoardBrief struct {
	ProjItemBrief
	Parameters Parameters `json:"parameters,omitempty" firestore:"parameters,omitempty"`
	// TODO: Document why and how to use, add tests
	RequiredParams [][]string `json:"requiredParams,omitempty" firestore:"requiredParams,omitempty"`
}

func (v ProjBoardBrief) Validate() error {
	if err := v.ValidateWithOptions(true); err != nil {
		return err
	}
	for i, p := range v.Parameters {
		if err := p.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("parameters", fmt.Sprintf("invalid parameter #%v: %v", i+1, err))
		}
	}
	return nil
}

//func validateBoardBriefsMappedByID(boards map[string]*ProjBoardBrief) error { // TODO: generic
//	for id, board := range boards {
//		if err := validateItemMappedByID(id, board.GetID, board); err != nil {
//			return err
//		}
//	}
//	return nil
//}

// ParameterLookup holds definition for parameter lookup
type ParameterLookup struct {
	DB        string   `json:"db" firestore:"db"`
	SQL       string   `json:"sql" firestore:"sql"`
	KeyFields []string `json:"keyFields" firestore:"keyFields"`
}

// BoardRows is slice of `BoardRow`
type BoardRows []BoardRow

// Validate returns error if failed
func (v BoardRows) Validate() error {
	for i, row := range v {
		if err := row.Validate(); err != nil {
			return fmt.Errorf("invalid row at index %v: %w", i, err)
		}
	}
	return nil
}

// BoardRow holds all details about a row in a board
type BoardRow struct {
	MinHeight string     `json:"minHeight,omitempty"`
	MaxHeight string     `json:"maxHeight,omitempty"`
	Cards     BoardCards `json:"cards,omitempty"`
}

// Validate returns error if failed
func (v BoardRow) Validate() error {
	// TODO: test MinHeight & MaxHeight
	if err := v.Cards.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("cards", err.Error())
	}
	return nil
}

// BoardCards is slice of BoardCard
type BoardCards []BoardCard

// Validate returns error if not valid
func (v BoardCards) Validate() error {
	for i, card := range v {
		if err := card.Validate(); err != nil {
			return fmt.Errorf("invalid card at index %v: %w", i, err)
		}
	}
	return nil
}

// BoardCard describes board card
type BoardCard struct {
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Cols   int          `json:"cols,omitempty"`
	Widget *BoardWidget `json:"widget,omitempty"`
	// Ref references a project card instead of holding an inline Widget.
	// Exactly one of Widget or Ref is set.
	Ref *BoardCardRef `json:"ref,omitempty"`
}

// Validate returns error if failed
func (v BoardCard) Validate() error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if v.Cols < 0 {
		return validation.NewErrBadRecordFieldValue("cols", "should be positive")
	}
	switch {
	case v.Widget != nil && v.Ref != nil:
		return validation.NewErrBadRecordFieldValue("ref", "widget and ref are mutually exclusive")
	case v.Ref != nil:
		if err := v.Ref.Validate(); err != nil {
			return fmt.Errorf("invalid card ref with id=%v: %w", v.ID, err)
		}
		return nil
	case v.Widget == nil:
		return validation.NewErrRecordIsMissingRequiredField("widget")
	}
	if err := v.Widget.Validate(); err != nil {
		return fmt.Errorf("invalid card widget with id=%v: %w", v.ID, err)
	}
	return nil
}

// BoardWidget specifies widget. Some widgets can contain otter widgets.
type BoardWidget struct {
	Name string      `json:"name"`
	Data interface{} `json:"data,omitempty"`
}

// Validate returns error if failed
func (v BoardWidget) Validate() (err error) {
	return v.validateAtDepth(0)
}

// maxWidgetValidationDepth also bounds cycles in programmatically built widgets.
const maxWidgetValidationDepth = 32

func (v BoardWidget) validateAtDepth(depth int) (err error) {
	if depth > maxWidgetValidationDepth {
		return validation.NewErrBadRecordFieldValue("tabs", "widget nesting exceeds maximum depth")
	}
	var widget interface{ Validate() error }
	var ok bool
	switch v.Name {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("name")
	case string(QueryTypeSQL): // Why switching name on a query type? Temporary demo hack?
		if widget, ok = v.Data.(*SQLWidgetDef); !ok {
			if widget, err = decodeSQLWidgetDef(v.Data); err != nil {
				return err
			}
		}
	case string(QueryTypeHTTP):
		if widget, ok = v.Data.(*HTTPWidgetDef); !ok {
			if widget, err = newWidgetDef(&HTTPWidgetDef{}, v.Data); err != nil {
				return err
			}
		}
	case "tabs":
		if typed, value := v.Data.(TabsWidgetDef); value {
			widget = &typed
		} else if widget, ok = v.Data.(*TabsWidgetDef); !ok {
			if widget, err = newWidgetDef(&TabsWidgetDef{}, v.Data); err != nil {
				return err
			}
		}
	case "chart":
		if typed, value := v.Data.(ChartWidgetDef); value {
			widget = &typed
		} else if widget, ok = v.Data.(*ChartWidgetDef); !ok {
			if widget, err = newStrictWidgetDef(&ChartWidgetDef{}, v.Data); err != nil {
				return err
			}
		}
	case "text":
		if typed, value := v.Data.(TextWidgetDef); value {
			widget = &typed
		} else if widget, ok = v.Data.(*TextWidgetDef); !ok {
			if widget, err = newStrictWidgetDef(&TextWidgetDef{}, v.Data); err != nil {
				return err
			}
		}
	case "fields":
		if typed, value := v.Data.(FieldsWidgetDef); value {
			widget = &typed
		} else if widget, ok = v.Data.(*FieldsWidgetDef); !ok {
			if widget, err = newStrictWidgetDef(&FieldsWidgetDef{}, v.Data); err != nil {
				return err
			}
		}
	default:
		return validation.NewErrBadRecordFieldValue("id", "unknown widget id")
	}
	if tabs, ok := widget.(*TabsWidgetDef); ok {
		if tabs == nil {
			// A nil definition is the same empty placeholder as absent JSON data.
			tabs = &TabsWidgetDef{}
		}
		return tabs.validateAtDepth(depth)
	}
	if sql, ok := widget.(*SQLWidgetDef); ok && sql == nil {
		return validation.NewErrBadRecordFieldValue("data", "SQL widget definition is null")
	}
	if http, ok := widget.(*HTTPWidgetDef); ok && http == nil {
		return validation.NewErrBadRecordFieldValue("data", "HTTP widget definition is null")
	}
	if chart, ok := widget.(*ChartWidgetDef); ok && chart == nil {
		return validation.NewErrBadRecordFieldValue("data", "chart widget definition is null")
	}
	if text, ok := widget.(*TextWidgetDef); ok && text == nil {
		return validation.NewErrBadRecordFieldValue("data", "text widget definition is null")
	}
	if fields, ok := widget.(*FieldsWidgetDef); ok && fields == nil {
		return validation.NewErrBadRecordFieldValue("data", "fields widget definition is null")
	}
	if err = widget.Validate(); err != nil {
		return fmt.Errorf("failed to test widget of type %T: %w", widget, err)
	}
	return nil
}

func newWidgetDef(widgetDef validatable, data interface{}) (validatable, error) {
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return widgetDef, json.Unmarshal(d, widgetDef)
}

// New chart wires reject extra fields so an untyped board read cannot carry
// result rows, execution state or a second, unsupported source contract into
// saved JSON. Older widget kinds retain their existing permissive behavior.
func newStrictWidgetDef(widgetDef validatable, data interface{}) (validatable, error) {
	d, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(d))
	decoder.DisallowUnknownFields()
	return widgetDef, decoder.Decode(widgetDef)
}

// WidgetBase is base struct for widgets
type WidgetBase struct {
	Title      string     `json:"title,omitempty"`
	Parameters Parameters `json:"parameters,omitempty"`
}

// Validate returns error if not valid
func (v WidgetBase) Validate(isTitleRequired bool) error {
	if err := validateStringField("title", v.Title, isTitleRequired, MaxTitleLength); err != nil {
		return err
	}
	if err := v.Parameters.Validate(); err != nil {
		return err
	}
	return nil
}

// SQLWidgetDef holds info about a widget that makes SQL queries
type SQLWidgetDef struct {
	WidgetBase
	SQL SQLWidgetSettings `json:"sql"`
	// Source, when set, takes the rows from a card parameter instead of a saved
	// query. It is exclusive with SQL, which then stays absent in JSON.
	Source *ParameterRowsSource `json:"source,omitempty"`
}

// MarshalJSON leaves "sql" out of a widget whose rows come from a parameter.
func (v SQLWidgetDef) MarshalJSON() ([]byte, error) {
	out := struct {
		WidgetBase
		SQL    *SQLWidgetSettings   `json:"sql,omitempty"`
		Source *ParameterRowsSource `json:"source,omitempty"`
	}{WidgetBase: v.WidgetBase, Source: v.Source}
	if v.Source == nil {
		out.SQL = &v.SQL
	}
	return json.Marshal(out)
}

// decodeSQLWidgetDef decodes a SQL widget. A widget with a source is a new
// shape and is decoded strictly, with "sql" refused; the saved-query shape keeps
// its permissive legacy decoding.
func decodeSQLWidgetDef(data interface{}) (validatable, error) {
	raw, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	var keys map[string]json.RawMessage
	if json.Unmarshal(raw, &keys) == nil {
		if _, hasSource := keys["source"]; hasSource {
			if _, hasSQL := keys["sql"]; hasSQL {
				return nil, validation.NewErrBadRecordFieldValue("source", "sql and source are mutually exclusive")
			}
			return newStrictWidgetDef(&SQLWidgetDef{}, data)
		}
	}
	return newWidgetDef(&SQLWidgetDef{}, data)
}

// SQLWidgetSettings binds a SQL widget to a query in the project's query
// library. It deliberately carries no query text and no execution target of
// its own: the text and the target come from the referenced QueryDef, and the
// environment is the viewer's choice at view time. Founder ruling 2026-09-09
// ("I'm Ok with the suggested option 1"). The field shape below is the coding
// agent's design, not the founder's.
type SQLWidgetSettings struct {
	// QueryID references a QueryDef by id within the same project.
	QueryID string `json:"queryId"`
	// Parameters binds the referenced query's parameters.
	Parameters WidgetParameterBindings `json:"parameters,omitempty"`
	// PresentationFilter narrows an already admitted, complete result in the
	// viewer. It is never sent to the referenced query as a parameter.
	PresentationFilter *PresentationFilter `json:"presentationFilter,omitempty"`
}

// PresentationFilter is deliberately closed even though legacy SQL widget
// fields retain their permissive JSON decoder.
type PresentationFilter struct {
	SelectionID string `json:"selectionId"`
	Column      string `json:"column"`
}

func (v *PresentationFilter) UnmarshalJSON(data []byte) error {
	type wire PresentationFilter
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = PresentationFilter(decoded)
	return nil
}

func (v PresentationFilter) Validate() error {
	if v.SelectionID == "" || strings.TrimSpace(v.SelectionID) != v.SelectionID {
		return validation.NewErrBadRecordFieldValue("selectionId", "must be nonempty without surrounding whitespace")
	}
	if strings.TrimSpace(v.Column) == "" {
		return validation.NewErrRecordIsMissingRequiredField("column")
	}
	return nil
}

func (v *SQLWidgetSettings) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name := range fields {
		if strings.EqualFold(name, "presentationFilter") && name != "presentationFilter" {
			return validation.NewErrBadRecordFieldValue("presentationFilter", "must use the canonical field name")
		}
	}
	if raw, present := fields["presentationFilter"]; present && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return validation.NewErrBadRecordFieldValue("presentationFilter", "must not be null")
	}
	type wire SQLWidgetSettings
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*v = SQLWidgetSettings(decoded)
	return nil
}

// Validate returns error if not valid
func (v SQLWidgetSettings) Validate() error {
	if v.QueryID == "" {
		return validation.NewErrRecordIsMissingRequiredField("queryId")
	}
	if err := v.Parameters.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("parameters", err.Error())
	}
	if v.PresentationFilter != nil {
		if err := v.PresentationFilter.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("presentationFilter", err.Error())
		}
	}
	return nil
}

// WidgetParameterBindings is a slice of WidgetParameterBinding.
type WidgetParameterBindings []WidgetParameterBinding

// Validate returns error if not valid
func (v WidgetParameterBindings) Validate() error {
	seen := make(map[string]bool, len(v))
	for i, binding := range v {
		if err := binding.Validate(); err != nil {
			return fmt.Errorf("invalid binding at index %v: %w", i, err)
		}
		if seen[binding.ID] {
			return fmt.Errorf("invalid binding at index %v: duplicate binding for parameter id=%v", i, binding.ID)
		}
		seen[binding.ID] = true
	}
	return nil
}

// WidgetParameterBinding binds one parameter of the referenced query to either
// a constant value or one of the board's own parameters.
type WidgetParameterBinding struct {
	// ID is the id of the referenced query's parameter.
	ID string `json:"id"`
	// Value is a constant value for the parameter, e.g. recorded by
	// "pin this result to a board". Mutually exclusive with BoardParameterID.
	Value interface{} `json:"value,omitempty"`
	// BoardParameterID takes the value from the board parameter with this id,
	// so a viewer changing a board parameter re-runs the widget's query.
	// Mutually exclusive with Value.
	BoardParameterID string `json:"boardParameterId,omitempty"`
}

// Validate returns error if not valid. Neither Value nor BoardParameterID
// being set is VALID: it means "use the query parameter's own default",
// i.e. ParameterDef.DefaultValue for the referenced query parameter.
func (v WidgetParameterBinding) Validate() error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if v.Value != nil && v.BoardParameterID != "" {
		return validation.NewErrBadRecordFieldValue("boardParameterId", "value and boardParameterId are mutually exclusive")
	}
	return nil
}

// Validate returns error if not valid
func (v *SQLWidgetDef) Validate() error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	if v.Source != nil {
		if v.SQL.QueryID != "" || len(v.SQL.Parameters) > 0 || v.SQL.PresentationFilter != nil {
			return validation.NewErrBadRecordFieldValue("source", "sql and source are mutually exclusive")
		}
		return v.Source.Validate()
	}
	if err := v.SQL.Validate(); err != nil {
		return err
	}
	return nil
}

// HTTPHeaders is a []HTTPHeaderItem
type HTTPHeaders []HTTPHeaderItem

// Validate returns error if not valid
func (v HTTPHeaders) Validate() error {
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid header item at index %v: %w", i, err)
		}
	}
	return nil
}

// HTTPHeaderItem describes an HTTP header item
type HTTPHeaderItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Validate returns error if not valid
func (v HTTPHeaderItem) Validate() error {
	if v.Name == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if v.Value == "" {
		return validation.NewErrRecordIsMissingRequiredField("value")
	}
	return nil
}

// HTTPRequest describes an HTTP request
type HTTPRequest struct {
	Method             string      `json:"method"`
	URL                string      `json:"url"`
	Protocol           string      `json:"protocol,omitempty"`
	Headers            HTTPHeaders `json:"headers,omitempty"`
	TimeoutThresholdMs int         `json:"timeoutThresholdMs,omitempty"` // in milliseconds
	Parameters         Parameters  `json:"parameters,omitempty"`
	Content            string      `json:"content,omitempty"`
}

// Validate returns error if not valid
func (v HTTPRequest) Validate() error {
	if v.URL == "" {
		return validation.NewErrRecordIsMissingRequiredField("url")
	}
	if v.TimeoutThresholdMs < 0 {
		return validation.NewErrBadRecordFieldValue("timeoutThresholdMs", "should be 0 or positive")
	}
	switch v.Method {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("method")
	case "GET", "HEAD", "OPTIONS":
		if v.Content != "" {
			return validation.NewErrBadRecordFieldValue("content", v.Method+"%v request can't have content")
		}
	case "POST", "PUT", "DELETE", "CONNECT", "TRACE", "PATCH":
		// According to https://developer.mozilla.org/en-US/docs/Web/HTTP/Methods
	default:
		return validation.NewErrBadRecordFieldValue("method", "unknown method: "+v.Method)
	}
	if err := v.Headers.Validate(); err != nil {
		return fmt.Errorf("invalid header: %w", err)
	}
	return nil
}

// HTTPWidgetDef holds info about a widget that makes HTTP requests
type HTTPWidgetDef struct {
	WidgetBase
	Request HTTPRequest `json:"request"`
}

// Validate returns error if not valid
func (v HTTPWidgetDef) Validate() error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	if err := v.Request.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("request", err.Error())
	}
	return nil
}

// TabWidget describes a tab widget
type TabWidget struct {
	// ID is a stable identity for newly authored tabs. Legacy tabs may omit it.
	ID     string      `json:"id,omitempty"`
	Title  string      `json:"title"`
	Widget BoardWidget `json:"widget,omitempty"`
	// Widgets are the tab's widgets, rendered in order; each may carry its
	// own title. The legacy single Widget still loads.
	Widgets []BoardWidget `json:"widgets,omitempty"`
}

// MarshalJSON leaves out the legacy single widget of a tab that has none, so a
// tab that holds only Widgets does not marshal as "widget":{"name":""}.
func (v TabWidget) MarshalJSON() ([]byte, error) {
	type plain struct {
		ID      string        `json:"id,omitempty"`
		Title   string        `json:"title"`
		Widget  *BoardWidget  `json:"widget,omitempty"`
		Widgets []BoardWidget `json:"widgets,omitempty"`
	}
	out := plain{ID: v.ID, Title: v.Title, Widgets: v.Widgets}
	if v.Widget.Name != "" || v.Widget.Data != nil {
		out.Widget = &v.Widget
	}
	return json.Marshal(out)
}

// TabsWidgetDef describes set of tab widgets
type TabsWidgetDef struct {
	WidgetBase
	Tabs []TabWidget `json:"tabs"`
}

// Validate returns error if not valid
func (v TabsWidgetDef) Validate() error {
	return v.validateAtDepth(0)
}

func (v TabsWidgetDef) validateAtDepth(depth int) error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	seenIDs := make(map[string]bool, len(v.Tabs))
	for i, tab := range v.Tabs {
		if tab.ID != "" {
			if strings.TrimSpace(tab.ID) != tab.ID {
				return fmt.Errorf("invalid tab at index %v: id must not have surrounding whitespace", i)
			}
			if seenIDs[tab.ID] {
				return fmt.Errorf("invalid tab at index %v: duplicate id=%q", i, tab.ID)
			}
			seenIDs[tab.ID] = true
		}
		for j, widget := range tab.Widgets {
			if err := widget.validateAtDepth(depth + 1); err != nil {
				return fmt.Errorf("invalid tab at index %v with title=%q: widget at index %v: %w", i, tab.Title, j, err)
			}
		}
		// The wire permits placeholder tabs without a widget. Existing boards
		// also use titles as display text, not a guaranteed unique identity.
		if tab.Widget.Name == "" && tab.Widget.Data == nil {
			continue
		}
		if err := tab.Widget.validateAtDepth(depth + 1); err != nil {
			return fmt.Errorf("invalid tab at index %v with title=%q: %w", i, tab.Title, err)
		}
	}
	return nil
}

// ChartWidgetDef stores presentation choices and saved-query references only.
// Results, execution receipts and access rights are resolved at view time.
type ChartWidgetDef struct {
	WidgetBase
	Sources     []ChartSourceBinding `json:"sources"`
	Chart       ChartSpec            `json:"chart"`
	DefaultView string               `json:"defaultView,omitempty"`
}

// ChartSourceBinding identifies an independently executed saved query.
// It never embeds a connection, query text, or a cached result.
type ChartSourceBinding struct {
	ID         string                  `json:"id"`
	QueryID    string                  `json:"queryId"`
	Parameters WidgetParameterBindings `json:"parameters,omitempty"`
	Transform  *ChartSourceTransform   `json:"transform,omitempty"`
}

func (v ChartSourceBinding) Validate() error {
	if v.ID == "" || strings.TrimSpace(v.ID) != v.ID {
		return validation.NewErrBadRecordFieldValue("id", "must be nonempty without surrounding whitespace")
	}
	if v.QueryID == "" {
		return validation.NewErrRecordIsMissingRequiredField("queryId")
	}
	if err := v.Parameters.Validate(); err != nil {
		return err
	}
	if v.Transform != nil {
		return v.Transform.Validate()
	}
	return nil
}

// ChartSourceTransform names a reusable runtime operation on an admitted result.
// Its input column names are explicit so execution never depends on a query ID.
type ChartSourceTransform struct {
	Name          string `json:"name"`
	OrderIDColumn string `json:"orderIdColumn"`
	DateColumn    string `json:"dateColumn"`
	CountryColumn string `json:"countryColumn"`
}

func (v ChartSourceTransform) Validate() error {
	if v.Name != "orders-monthly-country-count" {
		return validation.NewErrBadRecordFieldValue("name", "unsupported chart source transform")
	}
	if strings.TrimSpace(v.OrderIDColumn) == "" || strings.TrimSpace(v.DateColumn) == "" || strings.TrimSpace(v.CountryColumn) == "" {
		return validation.NewErrBadRecordFieldValue("transform", "orderIdColumn, dateColumn and countryColumn are required")
	}
	if v.OrderIDColumn == v.DateColumn || v.OrderIDColumn == v.CountryColumn || v.DateColumn == v.CountryColumn {
		return validation.NewErrBadRecordFieldValue("transform", "input columns must differ")
	}
	return nil
}

// ChartSpec maps columns in the admitted or transformed result to a chart.
// The monthly orders transform emits period/category/value rows.
type ChartSpec struct {
	Timing         *TimingChartSpec   `json:"timing,omitempty"`
	Timeline       *TimelineChartSpec `json:"timeline,omitempty"`
	Heatmap        *HeatmapChartSpec  `json:"heatmap,omitempty"`
	Type           string             `json:"type"`
	CategoryColumn string             `json:"categoryColumn,omitempty"`
	ValueColumn    string             `json:"valueColumn,omitempty"`
	Series         []ChartSeries      `json:"series,omitempty"`
	SelectionID    string             `json:"selectionId,omitempty"`
	PeriodColumn   string             `json:"periodColumn,omitempty"`
	Unit           string             `json:"unit,omitempty"`
	Grain          string             `json:"grain,omitempty"`
	OnDuplicate    string             `json:"onDuplicate,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	TopN           int                `json:"topN,omitempty"`
}

// ChartSeries maps one numeric column of a single admitted query result.
type ChartSeries struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	ValueColumn string `json:"valueColumn"`
}

func (v ChartSeries) Validate() error {
	if v.ID == "" || strings.TrimSpace(v.ID) != v.ID {
		return validation.NewErrBadRecordFieldValue("id", "must be nonempty without surrounding whitespace")
	}
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("title")
	}
	if strings.TrimSpace(v.ValueColumn) == "" {
		return validation.NewErrRecordIsMissingRequiredField("valueColumn")
	}
	return nil
}

// The new branch needs to distinguish an absent legacy valueColumn from an
// explicit empty/null field; both otherwise decode to Go's zero string.
func (v *ChartSpec) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var chartType string
	_ = json.Unmarshal(fields["type"], &chartType)
	if chartType == "timeline" || chartType == "heatmap" || chartType == "timing-bars" {
		for name := range fields {
			for _, forbidden := range []string{"categoryColumn", "valueColumn", "series", "periodColumn", "unit", "grain", "onDuplicate", "mode", "topN"} {
				if strings.EqualFold(name, forbidden) {
					return validation.NewErrBadRecordFieldValue(name, "must be omitted for evidence charts")
				}
			}
		}
	}
	valuePresent := false
	for name := range fields {
		switch {
		case (strings.EqualFold(name, "timeline") && name != "timeline") || (strings.EqualFold(name, "heatmap") && name != "heatmap") || (strings.EqualFold(name, "timing") && name != "timing"):
			return validation.NewErrBadRecordFieldValue(name, "must use the canonical field name")
		case strings.EqualFold(name, "series") && name != "series":
			return validation.NewErrBadRecordFieldValue("series", "must use the canonical field name")
		case strings.EqualFold(name, "selectionId") && name != "selectionId":
			return validation.NewErrBadRecordFieldValue("selectionId", "must use the canonical field name")
		case strings.EqualFold(name, "valueColumn"):
			// Keep legacy case-insensitive decoding, while detecting any spelling
			// of this field when the new series representation is also present.
			valuePresent = true
		}
	}
	if series, present := fields["series"]; present {
		if bytes.Equal(bytes.TrimSpace(series), []byte("null")) {
			return validation.NewErrBadRecordFieldValue("series", "must not be null")
		}
		if valuePresent {
			return validation.NewErrBadRecordFieldValue("valueColumn", "must be omitted when series is present")
		}
	}
	if selection, present := fields["selectionId"]; present {
		var id string
		if err := json.Unmarshal(selection, &id); err != nil || id == "" || strings.TrimSpace(id) != id {
			return validation.NewErrBadRecordFieldValue("selectionId", "must be nonempty without surrounding whitespace")
		}
	}
	type schema ChartSpec
	if err := jsonstrict.CheckNoDuplicateKeysForSchema(data, reflect.TypeOf(schema{})); err != nil {
		return err
	}
	type wire ChartSpec
	var decoded wire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = ChartSpec(decoded)
	return nil
}

func (v ChartSpec) Validate() error {
	if v.Type == "timeline" || v.Type == "heatmap" || v.Type == "timing-bars" {
		return v.validateEvidenceChart()
	}
	if v.Timeline != nil || v.Heatmap != nil || v.Timing != nil {
		return validation.NewErrBadRecordFieldValue("chart", "evidence chart fields require their matching type")
	}
	switch v.Type {
	case "bar":
		if v.PeriodColumn != "" {
			return validation.NewErrBadRecordFieldValue("periodColumn", "only bar-race uses a period column")
		}
		if v.Grain != "" || v.OnDuplicate != "" || v.Mode != "" || v.TopN != 0 {
			return validation.NewErrBadRecordFieldValue("type", "race options require bar-race")
		}
		if v.SelectionID != "" && strings.TrimSpace(v.SelectionID) != v.SelectionID {
			return validation.NewErrBadRecordFieldValue("selectionId", "must not have surrounding whitespace")
		}
		if v.Series != nil {
			if v.ValueColumn != "" {
				return validation.NewErrBadRecordFieldValue("valueColumn", "valueColumn and series are mutually exclusive")
			}
			if len(v.Series) < 2 || len(v.Series) > 8 {
				return validation.NewErrBadRecordFieldValue("series", "must contain from 2 to 8 series")
			}
			if strings.TrimSpace(v.Unit) == "" {
				return validation.NewErrRecordIsMissingRequiredField("unit")
			}
			seenIDs := make(map[string]bool, len(v.Series))
			seenColumns := make(map[string]bool, len(v.Series))
			for i, series := range v.Series {
				if err := series.Validate(); err != nil {
					return fmt.Errorf("invalid series at index %v: %w", i, err)
				}
				if seenIDs[series.ID] {
					return validation.NewErrBadRecordFieldValue("series", "duplicate id")
				}
				if seenColumns[series.ValueColumn] {
					return validation.NewErrBadRecordFieldValue("series", "duplicate valueColumn")
				}
				if series.ValueColumn == v.CategoryColumn {
					return validation.NewErrBadRecordFieldValue("series", "category and value columns must differ")
				}
				seenIDs[series.ID] = true
				seenColumns[series.ValueColumn] = true
			}
		}
	case "bar-race":
		if v.Series != nil || v.SelectionID != "" {
			return validation.NewErrBadRecordFieldValue("type", "series and selectionId require bar")
		}
		if strings.TrimSpace(v.PeriodColumn) == "" {
			return validation.NewErrRecordIsMissingRequiredField("periodColumn")
		}
		if strings.TrimSpace(v.Unit) == "" {
			return validation.NewErrRecordIsMissingRequiredField("unit")
		}
		if v.Grain != "month" {
			return validation.NewErrBadRecordFieldValue("grain", "bar-race currently requires month")
		}
		if v.OnDuplicate != "sum" && v.OnDuplicate != "reject" {
			return validation.NewErrBadRecordFieldValue("onDuplicate", "must be sum or reject")
		}
		if v.Mode != "period" && v.Mode != "cumulative" {
			return validation.NewErrBadRecordFieldValue("mode", "must be period or cumulative")
		}
		if v.TopN < 1 || v.TopN > 100 {
			return validation.NewErrBadRecordFieldValue("topN", "must be from 1 to 100")
		}
	default:
		return validation.NewErrBadRecordFieldValue("type", "must be bar or bar-race")
	}
	if strings.TrimSpace(v.CategoryColumn) == "" {
		return validation.NewErrRecordIsMissingRequiredField("categoryColumn")
	}
	if v.Series == nil && strings.TrimSpace(v.ValueColumn) == "" {
		return validation.NewErrRecordIsMissingRequiredField("valueColumn")
	}
	if v.ValueColumn != "" && (v.CategoryColumn == v.ValueColumn || (v.PeriodColumn != "" && (v.PeriodColumn == v.CategoryColumn || v.PeriodColumn == v.ValueColumn))) {
		return validation.NewErrBadRecordFieldValue("chart", "period, category and value columns must differ")
	}
	return nil
}

func (v ChartWidgetDef) Validate() error {
	if err := v.WidgetBase.Validate(false); err != nil {
		return err
	}
	if len(v.Sources) == 0 {
		return validation.NewErrRecordIsMissingRequiredField("sources")
	}
	seenIDs := make(map[string]bool, len(v.Sources))
	for i, source := range v.Sources {
		if err := source.Validate(); err != nil {
			return fmt.Errorf("invalid source at index %v: %w", i, err)
		}
		if seenIDs[source.ID] {
			return fmt.Errorf("invalid source at index %v: duplicate id=%q", i, source.ID)
		}
		seenIDs[source.ID] = true
	}
	if err := v.Chart.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("chart", err.Error())
	}
	if (v.Chart.Type == "bar-race" || v.Chart.Type == "timeline" || v.Chart.Type == "heatmap" || v.Chart.Type == "timing-bars" || v.Chart.Series != nil) && len(v.Sources) != 1 {
		return validation.NewErrBadRecordFieldValue("sources", "bar-race, evidence charts and multi-series bar require one source")
	}
	for i, source := range v.Sources {
		if (v.Chart.Series != nil || v.Chart.Type == "timeline" || v.Chart.Type == "heatmap" || v.Chart.Type == "timing-bars") && source.Transform != nil {
			return fmt.Errorf("invalid source at index %v: multi-series bar does not support a source transform", i)
		}
		if source.Transform != nil && source.Transform.Name == "orders-monthly-country-count" {
			if v.Chart.Type != "bar-race" || v.Chart.PeriodColumn != "period" || v.Chart.CategoryColumn != "category" || v.Chart.ValueColumn != "value" {
				return fmt.Errorf("invalid source at index %v: orders-monthly-country-count requires a bar-race mapped to period, category and value", i)
			}
			if v.Chart.OnDuplicate != "reject" {
				return fmt.Errorf("invalid source at index %v: orders-monthly-country-count requires rejecting duplicate output rows", i)
			}
		}
	}
	switch v.DefaultView {
	case "", "chart", "grid":
		return nil
	default:
		return validation.NewErrBadRecordFieldValue("defaultView", "must be chart or grid")
	}
}

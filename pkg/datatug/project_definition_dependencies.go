package datatug

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// ProjectDefinitionDependency preserves the source contract's identity. A host
// resolves it inside its authorized transaction: bare query IDs, Entity fields,
// scoped Meanings, connection declarations and source aliases are not synonyms.
// Data holds the named concrete source contract for lookup, target, federation,
// recordset or entity-source requests. It is private resolver input, not UI data.
type ProjectDefinitionDependency struct {
	Kind        string
	ID          string
	EntityField EntityFieldRef
	Meaning     MeaningReference
	Environment string
	Data        json.RawMessage
}

type projectDependencyCollector struct {
	requests       map[string]ProjectDefinitionDependency
	bytes, widgets int
}

var marshalProjectDependency = json.Marshal
var validateProjectDependencyMeaning = MeaningReference.Validate
var validateProjectDependencyEntityField = EntityFieldRef.Validate

func newProjectDependencyCollector() *projectDependencyCollector {
	return &projectDependencyCollector{requests: map[string]ProjectDefinitionDependency{}}
}
func (c *projectDependencyCollector) add(request ProjectDefinitionDependency) error {
	body, err := marshalProjectDependency(request)
	if err != nil {
		return ErrProjectUsageIncomplete
	}
	digest := sha256.Sum256(body)
	key := hex.EncodeToString(digest[:])
	if _, exists := c.requests[key]; exists {
		return nil
	}
	if len(c.requests) >= 10000 || len(body) > (4<<20)-c.bytes {
		return ErrProjectUsageIncomplete
	}
	c.bytes += len(body)
	request.Data = append(json.RawMessage(nil), request.Data...)
	c.requests[key] = request
	return nil
}
func (c *projectDependencyCollector) opaque(kind string, value any) error {
	body, err := marshalProjectDependency(value)
	if err != nil {
		return ErrProjectUsageIncomplete
	}
	return c.add(ProjectDefinitionDependency{Kind: kind, Data: body})
}
func (c *projectDependencyCollector) parameters(parameters Parameters) error {
	for _, parameter := range parameters {
		if parameter.Meta != nil {
			// ParameterDef.Validate does not currently validate this pair.
			if parameter.Meta.Validate() != nil {
				return ErrProjectUsageIncomplete
			}
			if err := c.add(ProjectDefinitionDependency{Kind: "entity-field", EntityField: *parameter.Meta}); err != nil {
				return err
			}
		}
		if parameter.Lookup != nil {
			if err := c.opaque("lookup", *parameter.Lookup); err != nil {
				return err
			}
		}
	}
	return nil
}
func (c *projectDependencyCollector) result() []ProjectDefinitionDependency {
	keys := make([]string, 0, len(c.requests))
	for key := range c.requests {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]ProjectDefinitionDependency, 0, len(keys))
	for _, key := range keys {
		out = append(out, c.requests[key])
	}
	return out
}

// These extractors cover typed outgoing definition requests only. Every request
// needs an explicit provider resolution, a proven external identity or an error;
// unresolved requests must never silently become an empty dependency list.
// Execution/history inventory and lifecycle permissions are separate concerns.
func ProjectQueryDependencies(query QueryDefWithFolderPath) ([]ProjectDefinitionDependency, error) {
	// Immutable native pins use the canonical empty root-folder spelling.
	// Folder resolution belongs to the caller; the query body still validates.
	if query.QueryDef.Validate() != nil || query.Type == "folder" {
		return nil, ErrProjectUsageIncomplete
	}
	c := newProjectDependencyCollector()
	if query.ConnectionID != "" {
		if err := c.add(ProjectDefinitionDependency{Kind: "connection", ID: query.ConnectionID}); err != nil {
			return nil, err
		}
	}
	if err := c.parameters(query.Parameters); err != nil {
		return nil, err
	}
	for _, recordset := range query.Recordsets {
		if recordset.JSONSchema != "" || len(recordset.Files) != 0 {
			if err := c.opaque("recordset", recordset); err != nil {
				return nil, err
			}
		}
		for _, key := range recordset.ForeignKeys {
			// ForeignKey's legacy JSON codec loses DBCollectionKey's private
			// identity fields. Use the named table wire, never its empty Ref:{}.
			if key == nil {
				return nil, ErrProjectUsageIncomplete
			}
			target, err := entityTableToWire(key.RefTable)
			if err != nil {
				return nil, ErrProjectUsageIncomplete
			}
			if err = c.opaque("foreign-key", struct {
				Recordset  string          `json:"recordset"`
				Target     entityTableWire `json:"target"`
				Columns    []string        `json:"columns"`
				RefColumns []string        `json:"refColumns"`
			}{recordset.ID, target, key.Columns, key.RefColumns}); err != nil {
				return nil, err
			}
		}
		for _, column := range recordset.Columns {
			if column.Meta != nil {
				if validateProjectDependencyEntityField(*column.Meta) != nil {
					return nil, ErrProjectUsageIncomplete
				}
				if err := c.add(ProjectDefinitionDependency{Kind: "entity-field", EntityField: *column.Meta}); err != nil {
					return nil, err
				}
			}
		}
	}
	if query.SemanticModel != nil {
		for _, concept := range query.SemanticModel.Meaning.Concepts {
			if concept.MeaningRef != nil {
				if validateProjectDependencyMeaning(*concept.MeaningRef) != nil {
					return nil, ErrProjectUsageIncomplete
				}
				if err := c.add(ProjectDefinitionDependency{Kind: "meaning", Meaning: *concept.MeaningRef}); err != nil {
					return nil, err
				}
			}
		}
	}
	if query.Capture != nil {
		if err := c.add(ProjectDefinitionDependency{Kind: "source", ID: query.Capture.Source, Environment: query.Capture.Environment}); err != nil {
			return nil, err
		}
	}
	for _, target := range query.Targets {
		if err := c.opaque("target", target); err != nil {
			return nil, err
		}
	}
	if query.Federation != nil {
		if err := c.opaque("federation", *query.Federation); err != nil {
			return nil, err
		}
	}
	if query.Type == QueryTypeTugQL {
		// Text/structured YAML sources must pass the owning TugQL parser and
		// source registry. Neither database names nor joins are project IDs.
		if err := c.opaque("query-source", struct {
			Type           QueryType `json:"type"`
			StructuredBody bool      `json:"structuredBody"`
			Text           string    `json:"text"`
		}{query.Type, query.StructuredBody, query.Text}); err != nil {
			return nil, err
		}
	}
	if len(query.RelationshipBindings) != 0 {
		if err := c.opaque("relationships", query.RelationshipBindings); err != nil {
			return nil, err
		}
	}
	return c.result(), nil
}

func ProjectBoardDependencies(board Board) ([]ProjectDefinitionDependency, error) {
	if board.Validate() != nil {
		return nil, ErrProjectUsageIncomplete
	}
	c := newProjectDependencyCollector()
	if err := c.parameters(board.Parameters); err != nil {
		return nil, err
	}
	for _, row := range board.Rows {
		for _, card := range row.Cards {
			if card.Ref != nil {
				if err := c.add(ProjectDefinitionDependency{Kind: "card", ID: card.Ref.CardID}); err != nil {
					return nil, err
				}
			}
			if card.Widget != nil {
				if err := c.widget(*card.Widget, 0); err != nil {
					return nil, err
				}
			}
		}
	}
	return c.result(), nil
}

func ProjectCardDependencies(card Card) ([]ProjectDefinitionDependency, error) {
	if card.Validate() != nil {
		return nil, ErrProjectUsageIncomplete
	}
	c := newProjectDependencyCollector()
	for _, parameter := range card.Parameters {
		if err := c.parameters(Parameters{parameter.ParameterDef}); err != nil {
			return nil, err
		}
		// Match is a permissive meaning/column display predicate. It has no
		// declared project Entity or Meaning scope; never reinterpret its names.
	}
	if card.Widget != nil {
		if err := c.widget(*card.Widget, 0); err != nil {
			return nil, err
		}
	}
	return c.result(), nil
}

func ProjectEntityDependencies(entity Entity) ([]ProjectDefinitionDependency, error) {
	if entity.Validate() != nil {
		return nil, ErrProjectUsageIncomplete
	}
	c := newProjectDependencyCollector()
	// Tables and field mappings use the owning source registry. Passing the
	// typed Entity preserves that context; source strings are not database IDs.
	physical := len(entity.Tables) != 0
	for _, field := range entity.Fields {
		physical = physical || len(field.Mappings) != 0
	}
	if physical {
		if err := c.opaque("entity-source", entity); err != nil {
			return nil, err
		}
	}
	return c.result(), nil
}

func (c *projectDependencyCollector) widget(widget BoardWidget, depth int) error {
	c.widgets++
	if depth > maxWidgetValidationDepth || c.widgets > 2000 {
		return ErrProjectUsageIncomplete
	}
	raw, err := json.Marshal(widget.Data)
	if err != nil || len(raw) > 1<<20 {
		return ErrProjectUsageIncomplete
	}
	var parameters Parameters
	switch widget.Name {
	case "SQL":
		type settings SQLWidgetSettings // bypass the permissive legacy decoder
		var value struct {
			WidgetBase
			SQL    settings             `json:"sql"`
			Source *ParameterRowsSource `json:"source,omitempty"`
		}
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
		if value.Source == nil {
			if err := c.add(ProjectDefinitionDependency{Kind: "query", ID: value.SQL.QueryID}); err != nil {
				return err
			}
		}
	case "chart":
		type chartSpec ChartSpec // unknown presentation/source fields stay unknown
		var value struct {
			WidgetBase
			Sources     []ChartSourceBinding `json:"sources"`
			Chart       chartSpec            `json:"chart"`
			DefaultView string               `json:"defaultView,omitempty"`
		}
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
		for _, source := range value.Sources {
			if err := c.add(ProjectDefinitionDependency{Kind: "query", ID: source.QueryID}); err != nil {
				return err
			}
		}
	case "tabs":
		var value TabsWidgetDef
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
		for _, tab := range value.Tabs {
			if tab.Widget.Name != "" || tab.Widget.Data != nil {
				if err := c.widget(tab.Widget, depth+1); err != nil {
					return err
				}
			}
			for _, child := range tab.Widgets {
				if err := c.widget(child, depth+1); err != nil {
					return err
				}
			}
		}
	case "fields":
		var value FieldsWidgetDef
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
		if value.Source.QueryID != "" {
			if err := c.add(ProjectDefinitionDependency{Kind: "query", ID: value.Source.QueryID}); err != nil {
				return err
			}
		}
	case "text":
		var value TextWidgetDef
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
	case "HTTP":
		var value HTTPWidgetDef
		if apicontract.DecodeStrict(raw, &value) != nil {
			return ErrProjectUsageIncomplete
		}
		parameters = value.Parameters
		if err := c.parameters(value.Request.Parameters); err != nil {
			return err
		}
	default:
		return ErrProjectUsageIncomplete
	}
	return c.parameters(parameters)
}

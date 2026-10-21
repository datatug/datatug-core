package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"gopkg.in/yaml.v3"
)

// These closed mapping objects share canonical required/optional field rules.
// A method-free target keeps duplicate-key detection aware of the exact schema.
func decodeClosedMapping(data []byte, target any) error {
	typ := reflect.TypeOf(target).Elem()
	if err := jsonstrict.CheckNoDuplicateKeysForSchema(data, typ); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return fmt.Errorf("mapping must be an object")
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		value, present := fields[name]
		if !present && options != "omitempty" {
			return fmt.Errorf("missing mapping field %s", name)
		}
		if present && bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("null mapping field %s", name)
		}
		delete(fields, name)
	}
	if len(fields) != 0 {
		return fmt.Errorf("unknown or noncanonical mapping field")
	}
	return json.Unmarshal(data, target)
}

func decodeMappingYAML(node *yaml.Node, target any) error {
	var raw any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return decodeClosedMapping(data, target)
}

func (v *QueryComparison) UnmarshalJSON(data []byte) error {
	type wire QueryComparison
	var decoded wire
	if err := decodeClosedMapping(data, &decoded); err != nil {
		return err
	}
	if err := QueryComparison(decoded).Validate(); err != nil {
		return err
	}
	*v = QueryComparison(decoded)
	return nil
}
func (v *QueryComparisonProjection) UnmarshalJSON(data []byte) error {
	type wire QueryComparisonProjection
	var decoded wire
	if err := decodeClosedMapping(data, &decoded); err != nil {
		return err
	}
	*v = QueryComparisonProjection(decoded)
	return nil
}
func (v *TimelineChartSpec) UnmarshalJSON(data []byte) error {
	type wire TimelineChartSpec
	var decoded wire
	if err := decodeClosedMapping(data, &decoded); err != nil {
		return err
	}
	if err := validateTimelineModeWire(data, TimelineChartSpec(decoded)); err != nil {
		return err
	}
	*v = TimelineChartSpec(decoded)
	return nil
}
func (v *QueryComparison) UnmarshalYAML(node *yaml.Node) error {
	type wire QueryComparison
	var decoded wire
	if err := decodeMappingYAML(node, &decoded); err != nil {
		return err
	}
	if err := QueryComparison(decoded).Validate(); err != nil {
		return err
	}
	*v = QueryComparison(decoded)
	return nil
}
func (v *QueryComparisonProjection) UnmarshalYAML(node *yaml.Node) error {
	type wire QueryComparisonProjection
	var decoded wire
	if err := decodeMappingYAML(node, &decoded); err != nil {
		return err
	}
	*v = QueryComparisonProjection(decoded)
	return nil
}
func (v *TimelineChartSpec) UnmarshalYAML(node *yaml.Node) error {
	var raw any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return v.UnmarshalJSON(data)
}

func validateTimelineModeWire(data []byte, value TimelineChartSpec) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, present := fields["timeMode"]; present && value.TimeMode == "" {
		return fmt.Errorf("empty timeline timeMode")
	}
	if _, present := fields["anchorColumn"]; present && value.TimeMode != "elapsed" {
		return fmt.Errorf("anchorColumn must be omitted unless elapsed")
	}
	return value.Validate()
}

// Query decoding stays strict inside outer strict save requests. Explicit
// envelopes avoid promoted methods losing folder scope; only comparison gains
// additional canonical/non-null checks on top of the existing field schema.
type queryDefWire QueryDef

type scopedQueryDefWire struct {
	FolderPath string `json:"folderPath"`
	queryDefWire
}

func checkQueryComparisonJSON(data []byte, schema reflect.Type) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, value := range fields {
		if strings.EqualFold(name, "comparison") && (name != "comparison" || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return fmt.Errorf("comparison must be canonical and nonnull")
		}
	}
	return jsonstrict.CheckNoDuplicateKeysForSchema(data, schema)
}
func (v *QueryDef) UnmarshalJSON(data []byte) error {
	if err := checkQueryComparisonJSON(data, reflect.TypeOf(queryDefWire{})); err != nil {
		return err
	}
	var decoded queryDefWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Preserve exact interface-valued parameter defaults.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = QueryDef(decoded)
	return nil
}
func (v *QueryDefWithFolderPath) UnmarshalJSON(data []byte) error {
	if err := checkQueryComparisonJSON(data, reflect.TypeOf(scopedQueryDefWire{})); err != nil {
		return err
	}
	var decoded scopedQueryDefWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber() // Preserve exact interface-valued parameter defaults.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = QueryDefWithFolderPath{FolderPath: decoded.FolderPath, QueryDef: QueryDef(decoded.queryDefWire)}
	return nil
}
func checkQueryComparisonYAML(node *yaml.Node) error {
	// Resolve aliases/merges so null cannot skip the comparison pointer codec.
	var raw any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	fields, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("query must be a YAML mapping")
	}
	for name, value := range fields {
		if strings.EqualFold(name, "comparison") && (name != "comparison" || value == nil) {
			return fmt.Errorf("comparison must be canonical and nonnull")
		}
	}
	return nil
}

func decodeKnownQueryYAML(node *yaml.Node, target any) error {
	// Preserve scalar Value/Tag rather than resolving into any (001 and dates
	// may be string fields). A nested custom decoder can reference an anchor
	// outside its subtree; detach only those aliases without changing their nodes.
	standalone, err := standaloneQueryYAML(node)
	if err != nil {
		return err
	}
	data, err := yaml.Marshal(standalone)
	if err != nil {
		return err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(target)
}

func standaloneQueryYAML(node *yaml.Node) (*yaml.Node, error) {
	members := map[*yaml.Node]bool{}
	var collect func(*yaml.Node)
	collect = func(n *yaml.Node) {
		if n == nil || members[n] {
			return
		}
		members[n] = true
		for _, child := range n.Content {
			collect(child)
		}
	}
	collect(node)
	// Anchor labels may be reused in valid YAML. Re-serializing an external
	// target can move an old label past a newer one, so labels must follow
	// original node identity rather than original spelling in the detached graph.
	anchors := map[*yaml.Node]string{}
	anchorName := func(n *yaml.Node) string {
		if name, ok := anchors[n]; ok {
			return name
		}
		name := fmt.Sprintf("query_anchor_%d", len(anchors)+1)
		anchors[n] = name
		return name
	}
	copies := map[*yaml.Node]*yaml.Node{}
	expanding := map[*yaml.Node]bool{}
	var clone func(*yaml.Node) (*yaml.Node, error)
	clone = func(n *yaml.Node) (*yaml.Node, error) {
		if n == nil {
			return nil, nil
		}
		if n.Kind == yaml.AliasNode && !members[n.Alias] {
			if n.Alias == nil || expanding[n.Alias] {
				return nil, fmt.Errorf("invalid or cyclic external YAML alias")
			}
			expanding[n.Alias] = true
			target, err := clone(n.Alias)
			delete(expanding, n.Alias)
			if err != nil {
				return nil, err
			}
			// This target's anchor lives outside the serialized subtree. Materialize
			// its unchanged scalar/tag/content; internal anchors and aliases remain.
			detached := *target
			detached.Anchor = ""
			return &detached, nil
		}
		if copied, ok := copies[n]; ok {
			return copied, nil
		}
		copied := *n
		if n.Anchor != "" {
			copied.Anchor = anchorName(n)
		}
		if n.Kind == yaml.AliasNode && n.Alias != nil {
			copied.Value = anchorName(n.Alias)
		}
		copies[n] = &copied
		copied.Content = make([]*yaml.Node, len(n.Content))
		for i, child := range n.Content {
			var err error
			copied.Content[i], err = clone(child)
			if err != nil {
				return nil, err
			}
		}
		if n.Alias != nil {
			var err error
			copied.Alias, err = clone(n.Alias)
			if err != nil {
				return nil, err
			}
		}
		return &copied, nil
	}
	return clone(node)
}

func (v *QueryDef) UnmarshalYAML(node *yaml.Node) error {
	if err := checkQueryComparisonYAML(node); err != nil {
		return err
	}
	var decoded queryDefWire
	if err := decodeKnownQueryYAML(node, &decoded); err != nil {
		return err
	}
	*v = QueryDef(decoded)
	return nil
}
func (v *QueryDefWithFolderPath) UnmarshalYAML(node *yaml.Node) error {
	// Preserve the existing YAML envelope's nested querydef (not JSON's flattening).
	var decoded struct {
		FolderPath string   `yaml:"folderpath"`
		QueryDef   QueryDef `yaml:"querydef"`
	}
	if err := decodeKnownQueryYAML(node, &decoded); err != nil {
		return err
	}
	*v = QueryDefWithFolderPath{FolderPath: decoded.FolderPath, QueryDef: decoded.QueryDef}
	return nil
}

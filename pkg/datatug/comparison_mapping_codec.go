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

func checkQueryComparisonJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	for name, value := range fields {
		if strings.EqualFold(name, "comparison") && (name != "comparison" || bytes.Equal(bytes.TrimSpace(value), []byte("null"))) {
			return fmt.Errorf("comparison must be canonical and nonnull")
		}
	}
	return jsonstrict.CheckNoDuplicateKeysForSchema(data, reflect.TypeOf(queryDefWire{}))
}
func (v *QueryDef) UnmarshalJSON(data []byte) error {
	if err := checkQueryComparisonJSON(data); err != nil {
		return err
	}
	var decoded queryDefWire
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = QueryDef(decoded)
	return nil
}
func (v *QueryDefWithFolderPath) UnmarshalJSON(data []byte) error {
	if err := checkQueryComparisonJSON(data); err != nil {
		return err
	}
	var decoded struct {
		FolderPath string `json:"folderPath"`
		queryDefWire
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*v = QueryDefWithFolderPath{FolderPath: decoded.FolderPath, QueryDef: QueryDef(decoded.queryDefWire)}
	return nil
}
func checkQueryComparisonYAML(node *yaml.Node) error {
	// Resolve YAML aliases/merges before testing presence: a null comparison
	// must not disappear when yaml skips the pointer's custom decoder.
	var raw any
	if err := node.Decode(&raw); err != nil {
		return err
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return checkQueryComparisonJSON(data)
}

func (v *QueryDef) UnmarshalYAML(node *yaml.Node) error {
	if err := checkQueryComparisonYAML(node); err != nil {
		return err
	}
	var decoded queryDefWire
	if err := node.Decode(&decoded); err != nil {
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
	if err := node.Decode(&decoded); err != nil {
		return err
	}
	*v = QueryDefWithFolderPath{FolderPath: decoded.FolderPath, QueryDef: decoded.QueryDef}
	return nil
}

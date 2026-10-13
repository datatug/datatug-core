package datatug

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

// QuerySemanticModel is an atomic, query-scoped pair of portable metadata documents.
// It contains schema and meaning only, never result values.
type QuerySemanticModel struct {
	Version  int                    `json:"version"`
	Basis    QuerySemanticBasis     `json:"basis"`
	Meaning  QueryMeaning           `json:"meaning"`
	Model    QueryModelSpec         `json:"model"`
	Bindings []QuerySemanticBinding `json:"bindings"`
}
type QuerySemanticBasis struct {
	QueryHash   string                `json:"queryHash"`
	ResultIndex int                   `json:"resultIndex"`
	Columns     []QuerySemanticColumn `json:"columns"`
}
type QuerySemanticColumn struct {
	Name     string `json:"name"`
	DataType string `json:"dataType"`
}
type QueryMeaning struct {
	Format      string                `json:"format"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Models      QueryMeaningModels    `json:"models"`
	Concepts    []QueryMeaningConcept `json:"concepts"`
}
type QueryMeaningModels struct {
	Query string `json:"query"`
}
type QueryMeaningLabels struct {
	En string `json:"en"`
}
type QueryMeaningConcept struct {
	ID          string                `json:"id"`
	Kind        string                `json:"kind"`
	Labels      QueryMeaningLabels    `json:"labels"`
	Description string                `json:"description"`
	Of          string                `json:"of,omitempty"`
	Bindings    []QueryMeaningBinding `json:"bindings,omitempty"`
}
type QueryMeaningBinding struct {
	Model string `json:"model"`
	Field string `json:"field,omitempty"`
	Role  string `json:"role"`
}
type QueryModelSpec struct {
	ModelSpec string            `json:"modelspec"`
	Module    QueryModelModule  `json:"module"`
	Records   QueryModelRecords `json:"records"`
}
type QueryModelModule struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
}
type QueryModelRecords struct {
	QueryResult QueryModelRecord `json:"QueryResult"`
}
type QueryModelRecord struct {
	Fields map[string]QueryModelField `json:"fields"`
}
type QueryModelField struct {
	Type string `json:"type"`
}
type QuerySemanticBinding struct {
	ColumnIndex int    `json:"columnIndex"`
	Field       string `json:"field"`
	Concept     string `json:"concept"`
	Role        string `json:"role,omitempty"`
}

// UnmarshalJSON refuses unknown metadata instead of silently losing it on save.
func (v *QuerySemanticModel) UnmarshalJSON(data []byte) error {
	type plain QuerySemanticModel
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := semanticRequired(raw, reflect.TypeOf(plain{})); err != nil {
		return err
	}
	var decoded plain
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&decoded); err != nil {
		return err
	}
	*v = QuerySemanticModel(decoded)
	return nil
}

var semanticHash = regexp.MustCompile(`^[a-f0-9]{64}$`)
var semanticID = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z][a-z0-9]*)*$`)
var semanticFileID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)
var semanticField = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

const querySemanticRecord = "modelspec:///query.QueryResult"

func (v QuerySemanticModel) Validate() error {
	bad := func(s string) error { return fmt.Errorf("invalid %s", s) }
	if v.Version != 1 || !semanticHash.MatchString(v.Basis.QueryHash) || v.Basis.ResultIndex < 0 || len(v.Basis.Columns) == 0 || len(v.Basis.Columns) > 512 {
		return bad("version or basis")
	}
	for _, c := range v.Basis.Columns {
		if strings.TrimSpace(c.Name) == "" || strings.TrimSpace(c.DataType) == "" {
			return bad("column")
		}
	}
	if v.Meaning.Format != "meaning/draft-2" || !semanticFileID.MatchString(v.Meaning.ID) || strings.TrimSpace(v.Meaning.Name) == "" || strings.TrimSpace(v.Meaning.Description) == "" || v.Meaning.Models.Query != "query.modelspec.json" || len(v.Meaning.Concepts) == 0 || len(v.Meaning.Concepts) > 1024 {
		return bad("meaning document")
	}
	if v.Model.ModelSpec != "1.0-draft-2" || v.Model.Module != (QueryModelModule{ID: "datatug/query", Name: "query", Version: "0.1.0"}) {
		return bad("ModelSpec module")
	}
	fields := v.Model.Records.QueryResult.Fields
	if len(fields) > 512 {
		return bad("field count")
	}
	for name, f := range fields {
		if !semanticField.MatchString(name) {
			return bad("field name")
		}
		switch f.Type {
		case "string", "int", "float", "bool", "decimal", "uuid", "date", "time", "datetime", "document", "json", "any":
		default:
			return bad("field type")
		}
	}
	concepts := make(map[string]QueryMeaningConcept, len(v.Meaning.Concepts))
	for _, c := range v.Meaning.Concepts {
		if len(c.ID) > 80 || !semanticID.MatchString(c.ID) || (c.Kind != "entity" && c.Kind != "property") || strings.TrimSpace(c.Labels.En) == "" || strings.TrimSpace(c.Description) == "" {
			return bad("concept")
		}
		if _, ok := concepts[c.ID]; ok {
			return bad("duplicate concept")
		}
		concepts[c.ID] = c
	}
	for _, c := range v.Meaning.Concepts {
		if c.Kind == "entity" {
			if c.Of != "" || len(c.Bindings) > 0 {
				return bad("entity assertion")
			}
		} else {
			parent, ok := concepts[c.Of]
			if !ok || parent.Kind != "entity" {
				return bad("property owner")
			}
		}
		seen := map[string]bool{}
		for _, b := range c.Bindings {
			if _, ok := fields[b.Field]; !ok || b.Model != querySemanticRecord || b.Role != "value" || seen[b.Field] {
				return bad("meaning binding")
			}
			seen[b.Field] = true
		}
	}
	if len(v.Bindings) > 512 {
		return bad("binding count")
	}
	columns := map[int]bool{}
	boundFields := map[string]bool{}
	for _, b := range v.Bindings {
		c, ok := concepts[b.Concept]
		if !ok || c.Kind != "property" || b.ColumnIndex < 0 || b.ColumnIndex >= len(v.Basis.Columns) || columns[b.ColumnIndex] || boundFields[b.Field] {
			return bad("column binding")
		}
		if _, ok := fields[b.Field]; !ok {
			return bad("binding field")
		}
		matched := false
		for _, mb := range c.Bindings {
			if mb.Field == b.Field {
				matched = true
			}
		}
		if !matched {
			return bad("missing meaning field binding")
		}
		columns[b.ColumnIndex] = true
		boundFields[b.Field] = true
	}
	return screenSemanticStrings(reflect.ValueOf(v))
}

// Reflection visits every string and map key without interpreting secret-named
// schema fields as sampled data. Existing credential policy screens each value.
func screenSemanticStrings(v reflect.Value) error {
	switch v.Kind() {
	case reflect.String:
		if reason, found := EmbeddedCredentialReason(v.String()); found {
			return fmt.Errorf("%s; %s", reason, storedQueryHint)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if err := screenSemanticStrings(v.Field(i)); err != nil {
				return err
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if err := screenSemanticStrings(v.Index(i)); err != nil {
				return err
			}
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			if err := screenSemanticStrings(key); err != nil {
				return err
			}
			if err := screenSemanticStrings(v.MapIndex(key)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Required zero-valued indices must be explicitly supplied on the wire.
func semanticRequired(raw any, typ reflect.Type) error {
	if raw == nil {
		return fmt.Errorf("semantic metadata cannot be null")
	}
	switch typ.Kind() {
	case reflect.Struct:
		obj, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expected semantic object")
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			parts := strings.Split(f.Tag.Get("json"), ",")
			name := parts[0]
			value, exists := obj[name]
			optional := len(parts) > 1 && parts[1] == "omitempty"
			if !exists {
				if optional {
					continue
				}
				return fmt.Errorf("missing semantic field %s", name)
			}
			if err := semanticRequired(value, f.Type); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if items, ok := raw.([]any); ok {
			for _, item := range items {
				if err := semanticRequired(item, typ.Elem()); err != nil {
					return err
				}
			}
		}
	case reflect.Map:
		if items, ok := raw.(map[string]any); ok {
			for _, item := range items {
				if err := semanticRequired(item, typ.Elem()); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

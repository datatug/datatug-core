package jsonstrict

import (
	"encoding/json"
	"reflect"
	"testing"
)

type embeddedTarget struct {
	Inner string `json:"inner"`
}

type objectTarget struct {
	Name   string                    `json:"name"`
	Items  []embeddedTarget          `json:"items"`
	Lookup map[string]embeddedTarget `json:"lookup"`
	Plain  string
	Skip   string `json:"-"`
	embeddedTarget
	hidden string
}

type customTarget struct{}

func (*customTarget) UnmarshalJSON([]byte) error { return nil }

func TestCheckNoDuplicateKeysFor(t *testing.T) {
	tests := []struct {
		name   string
		wire   string
		target reflect.Type
		valid  bool
	}{
		{name: "scalar", wire: `"value"`, valid: true},
		{name: "struct", wire: `{"name":"first","items":[{"inner":"x"}]}`, target: reflect.TypeOf(&objectTarget{}), valid: true},
		{name: "case folded duplicate", wire: `{"name":"first","Name":"last"}`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "exact duplicate", wire: `{"name":"first","name":"last"}`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "array element duplicate", wire: `{"items":[{"inner":"x","Inner":"y"}]}`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "map value duplicate", wire: `{"lookup":{"one":{"inner":"x","Inner":"y"}}}`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "map case variants remain distinct", wire: `{"lookup":{"one":{},"ONE":{}}}`, target: reflect.TypeOf(objectTarget{}), valid: true},
		{name: "unknown exact duplicate", wire: `{"outer":{"key":1,"key":2}}`, valid: false},
		{name: "unknown case variants remain distinct", wire: `{"outer":{"key":1,"KEY":2}}`, valid: true},
		{name: "missing root token", wire: ``, valid: false},
		{name: "missing object key", wire: `{"`, valid: false},
		{name: "missing object value", wire: `{"name":`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "missing object close", wire: `{"name":"x"`, target: reflect.TypeOf(objectTarget{}), valid: false},
		{name: "missing array close", wire: `[1`, target: reflect.TypeOf([]int{}), valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNoDuplicateKeysFor([]byte(tt.wire), tt.target)
			if (err == nil) != tt.valid {
				t.Fatalf("CheckNoDuplicateKeysFor() error = %v, valid = %v", err, tt.valid)
			}
		})
	}
}

func TestObjectFieldsAndFolding(t *testing.T) {
	_ = objectTarget{hidden: "ignored"}
	fields, ok := ObjectFields(reflect.TypeOf(objectTarget{}))
	if !ok {
		t.Fatal("struct fields were not detected")
	}
	for _, name := range []string{"name", "items", "lookup", "Plain", "inner"} {
		if fields[FoldName(name)] == nil {
			t.Fatalf("field %q was not detected", name)
		}
	}
	for _, name := range []string{"Skip", "hidden"} {
		if fields[FoldName(name)] != nil {
			t.Fatalf("field %q should not be decoded", name)
		}
	}
	for _, target := range []reflect.Type{nil, reflect.TypeOf(0), reflect.TypeOf(customTarget{})} {
		if _, ok := ObjectFields(target); ok {
			t.Fatalf("ObjectFields(%v) unexpectedly returned fields", target)
		}
	}
	if FoldName("project") != FoldName("PROJECT") || FoldName("k") != FoldName("K") {
		t.Fatal("JSON name folding does not match encoding/json")
	}
	if FoldRune('k') != 'K' {
		t.Fatal("ASCII case-fold orbit was not normalized")
	}
	var _ json.Unmarshaler = (*customTarget)(nil)
}

func TestCheckNoDuplicateKeysForSchema(t *testing.T) {
	for _, tc := range []struct {
		name, wire string
		valid      bool
	}{
		{"known aliases", `{"name":"first","Name":"last"}`, false},
		{"nested typed aliases", `{"items":[{"inner":"first","Inner":"last"}]}`, false},
		{"map typed aliases", `{"lookup":{"one":{"inner":"first","Inner":"last"}}}`, false},
		{"unknown root casing", `{"custom":null,"Custom":{"integer":9007199254740993}}`, true},
		{"unknown nested casing", `{"items":[{"custom":null,"Custom":{}}]}`, true},
		{"opaque nested casing", `{"custom":{"key":null,"KEY":1}}`, true},
		{"exact unknown duplicate", `{"custom":null,"custom":1}`, false},
		{"exact opaque nested duplicate", `{"custom":{"key":null,"key":1}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckNoDuplicateKeysForSchema([]byte(tc.wire), reflect.TypeOf(objectTarget{}))
			if (err == nil) != tc.valid {
				t.Fatalf("schema check error = %v, valid = %v", err, tc.valid)
			}
		})
	}
	// Existing callers still fold all struct-object names, including unknown ones.
	if err := CheckNoDuplicateKeysFor([]byte(`{"custom":null,"Custom":1}`), reflect.TypeOf(objectTarget{})); err == nil {
		t.Fatal("existing strict caller behavior changed")
	}
	// Opting into a schema does not reflect through custom unmarshalers implicitly.
	if err := CheckNoDuplicateKeysForSchema([]byte(`{"custom":null,"Custom":1}`), reflect.TypeOf(customTarget{})); err != nil {
		t.Fatal(err)
	}
}

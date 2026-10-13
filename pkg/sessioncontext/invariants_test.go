package sessioncontext

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

func TestDefinitionDocumentsAreCanonicalAndClosed(t *testing.T) {
	goodQuery := testQueryDefinition("select 1")
	goodDashboard := testDefinition("dashboard")
	goodChat := testDefinition("chat")
	for _, test := range []struct {
		name string
		kind string
		raw  json.RawMessage
	}{
		{name: "query", kind: "query", raw: goodQuery},
		{name: "dashboard", kind: "dashboard", raw: goodDashboard},
		{name: "chat", kind: "chat", raw: goodChat},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, digest, err := canonicalDefinitionDocument(test.kind, test.raw)
			if err != nil || !json.Valid(got) || digest == "" {
				t.Fatalf("canonical definition: %s %v", got, err)
			}
			if again, againDigest, err := canonicalDefinitionDocument(test.kind, got); err != nil || string(again) != string(got) || digest != againDigest {
				t.Fatalf("canonicalization is not stable: %s %s %v", again, againDigest, err)
			}
		})
	}
	invalid := []struct {
		name string
		kind string
		raw  json.RawMessage
	}{
		{"empty", "query", nil},
		{"oversize", "chat", json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[],"padding":"` + strings.Repeat("x", 128*1024) + `"}`)},
		{"malformed", "chat", json.RawMessage(`{"schemaVersion":`)},
		{"duplicate top-level key", "chat", json.RawMessage(`{"schemaVersion":1,"schemaVersion":1,"kind":"chat","queries":[]}`)},
		{"unknown secret field", "chat", json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[],"apiKey":"secret"}`)},
		{"wrong schema", "chat", json.RawMessage(`{"schemaVersion":2,"kind":"chat","queries":[]}`)},
		{"wrong expected kind", "query", goodChat},
		{"query cardinality", "query", json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[]}`)},
		{"dashboard no widgets", "dashboard", json.RawMessage(`{"schemaVersion":1,"kind":"dashboard","queries":[{"id":"q","queryType":"SQL","text":"select 1"}]}`)},
		{"chat widgets forbidden", "chat", json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[],"widgets":[{"id":"w","queryId":"q"}]}`)},
		{"too many queries", "chat", manyDefinitionItems("queries", 101)},
		{"too many widgets", "dashboard", manyDefinitionItems("widgets", 101)},
		{"invalid query id", "query", queryDefinition(`{"id":"bad id","queryType":"SQL","text":"x"}`)},
		{"duplicate query id", "chat", json.RawMessage(`{"schemaVersion":1,"kind":"chat","queries":[{"id":"q","queryType":"SQL","text":"x"},{"id":"q","queryType":"SQL","text":"y"}]}`)},
		{"empty query text", "query", queryDefinition(`{"id":"q","queryType":"SQL","text":""}`)},
		{"long query text", "query", queryDefinition(fmt.Sprintf(`{"id":"q","queryType":"SQL","text":%q}`, strings.Repeat("x", 100001)))},
		{"unsafe connection", "query", queryDefinition(`{"id":"q","queryType":"SQL","text":"x","connectionId":"../private"}`)},
		{"too many parameters", "query", queryWithParameters(101)},
		{"unsupported query type", "query", queryDefinition(`{"id":"q","queryType":"GRAPHQL","text":"x"}`)},
		{"invalid parameter id", "query", queryDefinition(`{"id":"q","queryType":"SQL","text":"x","parameters":[{"id":"bad id","type":"integer"}]}`)},
		{"duplicate parameter id", "query", queryDefinition(`{"id":"q","queryType":"SQL","text":"x","parameters":[{"id":"p","type":"integer"},{"id":"p","type":"integer"}]}`)},
		{"unsupported parameter type", "query", queryDefinition(`{"id":"q","queryType":"SQL","text":"x","parameters":[{"id":"p","type":"object"}]}`)},
		{"invalid widget id", "dashboard", json.RawMessage(`{"schemaVersion":1,"kind":"dashboard","queries":[{"id":"q","queryType":"SQL","text":"x"}],"widgets":[{"id":"bad id","queryId":"q"}]}`)},
		{"duplicate widget id", "dashboard", json.RawMessage(`{"schemaVersion":1,"kind":"dashboard","queries":[{"id":"q","queryType":"SQL","text":"x"}],"widgets":[{"id":"w","queryId":"q"},{"id":"w","queryId":"q"}]}`)},
		{"unknown widget query", "dashboard", json.RawMessage(`{"schemaVersion":1,"kind":"dashboard","queries":[{"id":"q","queryType":"SQL","text":"x"}],"widgets":[{"id":"w","queryId":"missing"}]}`)},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := canonicalDefinitionDocument(test.kind, test.raw); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid definition, got %v", err)
			}
		})
	}
}

func manyDefinitionItems(field string, count int) json.RawMessage {
	items := make([]map[string]string, count)
	for i := range items {
		items[i] = map[string]string{"id": fmt.Sprintf("i%d", i), "queryType": "SQL", "text": "select 1", "queryId": "q"}
	}
	if field == "widgets" {
		doc := map[string]any{"schemaVersion": 1, "kind": "dashboard", "queries": []any{map[string]any{"id": "q", "queryType": "SQL", "text": "select 1"}}, "widgets": items}
		data, _ := json.Marshal(doc)
		return data
	}
	doc := map[string]any{"schemaVersion": 1, "kind": "chat", "queries": items}
	data, _ := json.Marshal(doc)
	return data
}

func queryDefinition(query string) json.RawMessage {
	return json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[` + query + `]}`)
}

func queryWithParameters(count int) json.RawMessage {
	parameters := make([]map[string]string, count)
	for i := range parameters {
		parameters[i] = map[string]string{"id": fmt.Sprintf("p%d", i), "type": "integer"}
	}
	data, _ := json.Marshal(map[string]any{"schemaVersion": 1, "kind": "query", "queries": []any{map[string]any{"id": "q", "queryType": "SQL", "text": "select 1", "parameters": parameters}}})
	return data
}

func TestTypedValueEnvelopeValidationAndCounts(t *testing.T) {
	validScalar := ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("42"))}
	validValues := ValueEnvelope{SchemaVersion: 1, Kind: "set", Values: []apicontract.TypedValue{apicontract.NewStringValue("a"), apicontract.NewStringValue("b")}}
	validComposite := ValueEnvelope{SchemaVersion: 1, Kind: "set", KeyFields: []string{"Tenant", "OrderID"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "Tenant", Value: apicontract.NewStringValue("acme")}, {Field: "OrderID", Value: apicontract.NewIntegerValue("42")}}}}}
	for name, value := range map[string]ValueEnvelope{"scalar": validScalar, "values": validValues, "composite": validComposite, "empty set": {SchemaVersion: 1, Kind: "set"}} {
		t.Run(name, func(t *testing.T) {
			if err := validateValue(value); err != nil {
				t.Fatalf("valid value rejected: %v", err)
			}
		})
	}
	if valueCount(validScalar) != 1 || valueCount(validValues) != 2 || valueCount(validComposite) != 1 || valueCount(ValueEnvelope{Kind: "set"}) != 0 {
		t.Fatal("value count does not reflect scalar, set, composite, or empty values")
	}
	invalid := map[string]ValueEnvelope{
		"wrong schema":              {SchemaVersion: 2, Kind: "set"},
		"missing scalar":            {SchemaVersion: 1, Kind: "scalar"},
		"scalar with set values":    {SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("x")), Values: []apicontract.TypedValue{apicontract.NewStringValue("y")}},
		"scalar with key fields":    {SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewStringValue("x")), KeyFields: []string{"id"}},
		"invalid typed scalar":      {SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("bad"))},
		"unknown kind":              {SchemaVersion: 1, Kind: "map"},
		"set with scalar":           {SchemaVersion: 1, Kind: "set", Scalar: valuePtr(apicontract.NewStringValue("x"))},
		"values and key fields":     {SchemaVersion: 1, Kind: "set", Values: []apicontract.TypedValue{apicontract.NewStringValue("x")}, KeyFields: []string{"id"}},
		"duplicate key field":       {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id", "id"}},
		"unsafe key field":          {SchemaVersion: 1, Kind: "set", KeyFields: []string{"bad field"}},
		"key missing field":         {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "other", Value: apicontract.NewIntegerValue("1")}}}}},
		"invalid key component":     {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: []EntityKey{{Components: []KeyComponent{{Field: "id", Value: apicontract.NewIntegerValue("bad")}}}}},
		"value list too large":      {SchemaVersion: 1, Kind: "set", Values: repeatedTypedValue(5301)},
		"key list too large":        {SchemaVersion: 1, Kind: "set", KeyFields: []string{"id"}, Keys: repeatedEntityKey(5301)},
		"serialized envelope large": {SchemaVersion: 1, Kind: "set", Values: []apicontract.TypedValue{apicontract.NewStringValue(strings.Repeat("x", 700000))}},
	}
	for name, value := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := validateValue(value); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid value, got %v", err)
			}
		})
	}
}

func repeatedTypedValue(count int) []apicontract.TypedValue {
	values := make([]apicontract.TypedValue, count)
	for i := range values {
		values[i] = apicontract.NewIntegerValue(fmt.Sprint(i))
	}
	return values
}

func repeatedEntityKey(count int) []EntityKey {
	keys := make([]EntityKey, count)
	for i := range keys {
		keys[i] = EntityKey{Components: []KeyComponent{{Field: "id", Value: apicontract.NewIntegerValue(fmt.Sprint(i))}}}
	}
	return keys
}

func TestSessionContextSmallValidationHelpers(t *testing.T) {
	for _, value := range []string{"string", "number", "integer", "decimal", "boolean", "date", "datetime"} {
		if !validParameterType(value) {
			t.Errorf("valid parameter type %q rejected", value)
		}
	}
	if validParameterType("object") {
		t.Fatal("unknown parameter type accepted")
	}
	if err := settlePendingRun(&Session{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("settling zero pending runs: %v", err)
	}
	session := Session{PendingRunCount: 1}
	if err := settlePendingRun(&session); err != nil || session.PendingRunCount != 0 {
		t.Fatalf("settling a pending run: count=%d err=%v", session.PendingRunCount, err)
	}
	if !safeCode("SOURCE_UNAVAILABLE") || safeCode("bad code") || safeCode("") || safeCode(strings.Repeat("A", 65)) {
		t.Fatal("error-code allowlist accepted or rejected an invalid value")
	}
	valid := []string{"completed", "failed", "canceled"}
	for _, state := range valid {
		code := ""
		if state == "failed" {
			code = "SOURCE_UNAVAILABLE"
		}
		if err := validateRunOutcome(state, []WidgetOutcome{{ID: "w1", State: "completed", ResultRef: "result-1"}, {ID: "w2", State: "failed", ErrorCode: "WIDGET_FAILED"}}, []string{"result-1"}, code); err != nil {
			t.Errorf("valid %s outcome rejected: %v", state, err)
		}
	}
	invalidOutcomes := []struct {
		state   string
		widgets []WidgetOutcome
		refs    []string
		code    string
	}{
		{"running", nil, nil, ""},
		{"failed", nil, nil, ""},
		{"completed", nil, nil, "ERROR"},
		{"failed", nil, nil, "lowercase"},
		{"completed", []WidgetOutcome{{ID: "bad id", State: "completed", ResultRef: "r"}}, nil, ""},
		{"completed", []WidgetOutcome{{ID: "w", State: "completed", ResultRef: "r"}, {ID: "w", State: "completed", ResultRef: "r2"}}, nil, ""},
		{"completed", []WidgetOutcome{{ID: "w", State: "running"}}, nil, ""},
		{"completed", []WidgetOutcome{{ID: "w", State: "failed"}}, nil, ""},
		{"completed", []WidgetOutcome{{ID: "w", State: "failed", ErrorCode: "bad code"}}, nil, ""},
		{"completed", []WidgetOutcome{{ID: "w", State: "completed"}}, nil, ""},
		{"completed", nil, []string{"bad ref"}, ""},
	}
	for i, outcome := range invalidOutcomes {
		if err := validateRunOutcome(outcome.state, outcome.widgets, outcome.refs, outcome.code); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid outcome %d accepted: %v", i, err)
		}
	}
	if err := validateRunOutcome("completed", make([]WidgetOutcome, 1001), nil, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversize widget outcomes accepted: %v", err)
	}
	if err := validateRunOutcome("completed", nil, make([]string, 1001), ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("oversize result refs accepted: %v", err)
	}
}

func TestResolvedParametersAreValidatedBeforeRetention(t *testing.T) {
	if err := validateResolvedParameters(map[string]apicontract.TypedValue{"OrderID": apicontract.NewIntegerValue("123")}); err != nil {
		t.Fatal(err)
	}
	invalid := []map[string]apicontract.TypedValue{
		{"": apicontract.NewIntegerValue("1")},
		{strings.Repeat("x", 129): apicontract.NewStringValue("x")},
		{"bad name": apicontract.NewStringValue("x")},
		{"p": apicontract.NewIntegerValue("bad")},
	}
	tooMany := make(map[string]apicontract.TypedValue, 101)
	for i := 0; i < 101; i++ {
		tooMany[fmt.Sprintf("p%d", i)] = apicontract.NewIntegerValue(fmt.Sprint(i))
	}
	invalid = append(invalid, tooMany)
	for i, parameters := range invalid {
		if err := validateResolvedParameters(parameters); !errors.Is(err, ErrInvalid) {
			t.Errorf("invalid parameter set %d accepted: %v", i, err)
		}
	}
	if _, _, err := canonicalDefinitionDocument("query", nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty definition accepted")
	}
	if _, _, err := canonicalDefinitionDocument("query", json.RawMessage(strings.Repeat("x", 128*1024+1))); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized definition accepted")
	}
	if _, _, err := canonicalDefinitionDocument("query", json.RawMessage(`{"schemaVersion":1,"kind":"query","queries":[{"id":"q","queryType":"SQL","text":"select 1"}]} {}`)); !errors.Is(err, ErrInvalid) {
		t.Fatal("trailing JSON accepted")
	}
}

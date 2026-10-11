package bridge

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/recordsetcompare"
)

const validRequest = `{"abi":"recordsetcompare/1","left":{"columns":[{"name":"id","type":"integer"},{"name":"value","type":"string"}],"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"before"}]]},"right":{"columns":[{"name":"id","type":"integer"},{"name":"value","type":"string"}],"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"after"}]]},"leftInfo":{"rowCount":1,"limitations":[]},"rightInfo":{"rowCount":1,"limitations":[]},"key":["id"]}`

func TestHandleJSONComparesWithoutReceiptsAndPreservesExactIntegerText(t *testing.T) {
	input := strings.Replace(validRequest, `"value":"1"`, `"value":"9007199254740993"`, 1)
	input = strings.Replace(input, `"value":"1"`, `"value":"9007199254740993"`, 1)
	var got response
	if err := json.Unmarshal([]byte(HandleJSON(input)), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ABI != ABI || !got.OK || got.Error != nil || got.Diff == nil {
		t.Fatalf("unexpected response: %+v", got)
	}
	if got.Diff.Summary.Changed != 1 {
		t.Fatalf("changed rows = %d, want 1", got.Diff.Summary.Changed)
	}
	if got.Diff.Changed[0].Key[0].Str != "9007199254740993" {
		t.Fatalf("key crossed JSON as %q", got.Diff.Changed[0].Key[0].Str)
	}
}

func TestHandleJSONAllowsEmptyRecordsets(t *testing.T) {
	input := strings.ReplaceAll(validRequest, `"rowCount":1`, `"rowCount":0`)
	input = strings.ReplaceAll(input, `"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"before"}]]`, `"rows":[]`)
	input = strings.ReplaceAll(input, `"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"after"}]]`, `"rows":[]`)
	var got response
	if err := json.Unmarshal([]byte(HandleJSON(input)), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Diff == nil || len(got.Diff.Added) != 0 || len(got.Diff.Removed) != 0 || len(got.Diff.Changed) != 0 {
		t.Fatalf("empty recordsets were rejected: %+v", got)
	}
}

func TestHandleJSONPreservesTypedValueJSONKinds(t *testing.T) {
	columns := []apicontract.Column{
		{Name: "id", Type: "integer"}, {Name: "amount", Type: "number"},
		{Name: "active", Type: "boolean"}, {Name: "optional", Type: "string"},
	}
	row := []apicontract.TypedValue{
		apicontract.NewIntegerValue("7"), apicontract.NewNumberValue(1.25),
		apicontract.NewBooleanValue(true), apicontract.NewNullValue(),
	}
	encoded, err := json.Marshal(request{
		ABI:       ABI,
		Left:      apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{row}},
		Right:     apicontract.Recordset{Columns: columns, Rows: [][]apicontract.TypedValue{row}},
		LeftInfo:  apicontract.CompareSideInfo{RowCount: 1, Limitations: []apicontract.Limitation{}},
		RightInfo: apicontract.CompareSideInfo{RowCount: 1, Limitations: []apicontract.Limitation{}},
		Key:       []string{"id"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got response
	if err := json.Unmarshal([]byte(HandleJSON(string(encoded))), &got); err != nil {
		t.Fatal(err)
	}
	if !got.OK || got.Diff == nil || got.Diff.Summary.Unchanged != 1 {
		t.Fatalf("typed-value JSON variants rejected: %+v", got)
	}
}

func TestCheckFieldsReturnsTypedValueValidationError(t *testing.T) {
	if err := checkFields(map[string]any{"type": "integer", "value": nil}, "typedValue"); err == nil {
		t.Fatal("typed-value discriminator mismatch was accepted")
	}
	if err := checkTypedValueKind(map[string]any{"type": nil, "value": nil}); err == nil {
		t.Fatal("non-string typed-value discriminator was accepted")
	}
}

func TestHandleJSONRejectsMalformedAndNonCanonicalRequests(t *testing.T) {
	tests := []struct {
		name, input, code string
	}{
		{"empty", "", "invalid_request"},
		{"array root", "[]", "invalid_request"},
		{"null root", "null", "invalid_request"},
		{"malformed", "{", "invalid_json"},
		{"trailing value", validRequest + ` {}`, "invalid_json"},
		{"unknown field", strings.Replace(validRequest, `"key":[`, `"extra":true,"key":[`, 1), "invalid_request"},
		{"folded field", strings.Replace(validRequest, `"abi":`, `"ABI":`, 1), "invalid_request"},
		{"duplicate field", strings.Replace(validRequest, `"key":[`, `"key":[],"key":[`, 1), "invalid_request"},
		{"missing field", strings.Replace(validRequest, `,"key":["id"]`, "", 1), "invalid_request"},
		{"wrong ABI", strings.Replace(validRequest, ABI, "recordsetcompare/2", 1), "invalid_abi"},
		{"null recordset", strings.Replace(validRequest, `"left":{"columns":[{"name":"id","type":"integer"},{"name":"value","type":"string"}],"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"before"}]]}`, `"left":null`, 1), "invalid_request"},
		{"null side info", strings.Replace(validRequest, `"leftInfo":{"rowCount":1,"limitations":[]}`, `"leftInfo":null`, 1), "invalid_request"},
		{"null rows", strings.Replace(validRequest, `"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"before"}]]`, `"rows":null`, 1), "invalid_request"},
		{"null row", strings.Replace(validRequest, `"rows":[[{"type":"integer","value":"1"},{"type":"string","value":"before"}]]`, `"rows":[null]`, 1), "invalid_request"},
		{"null columns", strings.Replace(validRequest, `"columns":[{"name":"id","type":"integer"},{"name":"value","type":"string"}]`, `"columns":null`, 1), "invalid_request"},
		{"null limitations", strings.Replace(validRequest, `"limitations":[]`, `"limitations":null`, 1), "invalid_request"},
		{"malformed typed value", strings.Replace(validRequest, `"value":"before"`, `"value":false`, 1), "invalid_request"},
		{"limit overrun", strings.TrimSuffix(validRequest, "}") + `,"limit":101}`, "limit_exceeded"},
		{"mismatched side count", strings.Replace(validRequest, `"rowCount":1`, `"rowCount":2`, 1), "compare_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got response
			if err := json.Unmarshal([]byte(HandleJSON(test.input)), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if got.OK || got.Error == nil || got.Error.Code != test.code {
				t.Fatalf("response = %+v, want error code %q", got, test.code)
			}
			if len(got.Error.Message) > MaxErrorMessage {
				t.Fatalf("error message has %d bytes, limit %d", len(got.Error.Message), MaxErrorMessage)
			}
		})
	}
}

func TestHandleRejectsInputAndNestingBounds(t *testing.T) {
	if _, err := handle([]byte(strings.Repeat(" ", MaxInputBytes+1))); errorCode(err) != "input_too_large" {
		t.Fatalf("oversized input error = %v", err)
	}
	deep := `{"abi":"recordsetcompare/1","left":{"columns":[],"rows":[]},"right":{"columns":[],"rows":[]},"leftInfo":{"rowCount":0,"limitations":[]},"rightInfo":{"rowCount":0,"limitations":[]},"key":[[[[[[[[[[[[[[[[[[[0]]]]]]]]]]]]]]]]]]}`
	if _, err := handle([]byte(deep)); errorCode(err) != "invalid_json" {
		t.Fatalf("deep input error = %v", err)
	}
	if err := checkJSONDepth([]byte(`{} @`)); err == nil {
		t.Fatal("malformed trailing bytes were accepted")
	}
	decoder := json.NewDecoder(strings.NewReader("["))
	if err := scanJSONValue(decoder, 0); err == nil {
		t.Fatal("incomplete array was accepted")
	}
	if _, err := decodeRawJSON([]byte("{")); err == nil {
		t.Fatal("malformed raw JSON was accepted")
	}
}

func TestHandleReportsResponseEncodingFailure(t *testing.T) {
	_, err := handleWithEncoder([]byte(validRequest), func(recordsetcompare.Diff) ([]byte, error) {
		return nil, errors.New("injected encoder error")
	})
	if errorCode(err) != "response_invalid" {
		t.Fatalf("encoding failure = %v", err)
	}
}

func TestBridgeErrorAndFallbackEncoding(t *testing.T) {
	if got := (&bridgeError{code: "example", message: "message"}).Error(); got != "example: message" {
		t.Fatalf("error text = %q", got)
	}
	if got := boundedMessage("short"); got != "short" {
		t.Fatalf("short message = %q", got)
	}
	if got := boundedMessage(strings.Repeat("x", MaxErrorMessage+1)); len(got) != MaxErrorMessage {
		t.Fatalf("bounded message length = %d", len(got))
	}
	var fallback response
	if err := json.Unmarshal([]byte(encodeError(errors.New("untyped failure"), MaxOutputBytes)), &fallback); err != nil {
		t.Fatal(err)
	}
	if fallback.Error == nil || fallback.Error.Code != "invalid_request" {
		t.Fatalf("generic error response = %+v", fallback)
	}
	if got := encodeError(fail("oversized", strings.Repeat("x", MaxErrorMessage+1)), 1); !strings.Contains(got, `"internal_error"`) {
		t.Fatalf("fallback error response = %q", got)
	}
}

func TestHandleRejectsOversizedCompleteOutput(t *testing.T) {
	left := apicontract.Recordset{
		Columns: []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "value", Type: "string"}},
		Rows:    make([][]apicontract.TypedValue, 45),
	}
	right := apicontract.Recordset{
		Columns: []apicontract.Column{{Name: "id", Type: "integer"}, {Name: "value", Type: "string"}},
		Rows:    make([][]apicontract.TypedValue, 45),
	}
	for i := range left.Rows {
		id := strconv.Itoa(i)
		left.Rows[i] = []apicontract.TypedValue{apicontract.NewIntegerValue(id), apicontract.NewStringValue(strings.Repeat("<", 1_500))}
		right.Rows[i] = []apicontract.TypedValue{apicontract.NewIntegerValue(id), apicontract.NewStringValue(strings.Repeat(">", 1_500))}
	}
	req := request{
		ABI: ABI, Left: left, Right: right,
		LeftInfo:  apicontract.CompareSideInfo{RowCount: len(left.Rows), Limitations: []apicontract.Limitation{}},
		RightInfo: apicontract.CompareSideInfo{RowCount: len(right.Rows), Limitations: []apicontract.Limitation{}},
		Key:       []string{"id"},
	}
	input, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	if len(input) > MaxInputBytes {
		t.Fatalf("test request has %d bytes, exceeds input limit %d", len(input), MaxInputBytes)
	}
	if _, err := handle(input); errorCode(err) != "output_too_large" {
		t.Fatalf("oversized complete output error = %v", err)
	}
}

func TestValidateRequestBounds(t *testing.T) {
	var req request
	if err := json.Unmarshal([]byte(validRequest), &req); err != nil {
		t.Fatal(err)
	}
	if err := validateRequestBounds(req); err != nil {
		t.Fatalf("valid request bounds: %v", err)
	}
	tests := []struct {
		name string
		edit func(*request)
		code string
	}{
		{"missing ABI", func(r *request) { r.ABI = "" }, "invalid_request"},
		{"missing key", func(r *request) { r.Key = nil }, "invalid_request"},
		{"too many keys", func(r *request) { r.Key = make([]string, MaxKeyColumns+1) }, "invalid_request"},
		{"empty key name", func(r *request) { r.Key = []string{""} }, "invalid_request"},
		{"long key name", func(r *request) { r.Key = []string{strings.Repeat("k", MaxNameBytes+1)} }, "invalid_request"},
		{"long distribution name", func(r *request) { r.DistributionColumn = strings.Repeat("d", maxDistributionCol+1) }, "invalid_request"},
		{"negative limit", func(r *request) { r.Limit = -1 }, "limit_exceeded"},
		{"limit over max", func(r *request) { r.Limit = MaxRowsPerSide + 1 }, "limit_exceeded"},
		{"too many rows", func(r *request) { r.Right.Rows = make([][]apicontract.TypedValue, MaxRowsPerSide+1) }, "limit_exceeded"},
		{"too many left rows", func(r *request) { r.Left.Rows = make([][]apicontract.TypedValue, MaxRowsPerSide+1) }, "limit_exceeded"},
		{"too many columns", func(r *request) { r.Right.Columns = make([]apicontract.Column, MaxColumns+1) }, "limit_exceeded"},
		{"long column name", func(r *request) { r.Right.Columns[0].Name = strings.Repeat("n", MaxNameBytes+1) }, "limit_exceeded"},
		{"long column type", func(r *request) { r.Right.Columns[0].Type = strings.Repeat("t", 33) }, "limit_exceeded"},
		{"long cell", func(r *request) { r.Right.Rows[0][0].Str = strings.Repeat("x", MaxCellBytes+1) }, "limit_exceeded"},
		{"long policy", func(r *request) {
			r.LeftInfo.Limitations = []apicontract.Limitation{{Policy: strings.Repeat("p", MaxNameBytes+1)}}
		}, "limit_exceeded"},
		{"long right policy", func(r *request) {
			r.RightInfo.Limitations = []apicontract.Limitation{{Policy: strings.Repeat("p", MaxNameBytes+1)}}
		}, "limit_exceeded"},
		{"too many hidden", func(r *request) {
			r.LeftInfo.Limitations = []apicontract.Limitation{{Policy: "policy", HiddenColumns: make([]string, MaxColumns+1)}}
		}, "limit_exceeded"},
		{"long hidden name", func(r *request) {
			r.LeftInfo.Limitations = []apicontract.Limitation{{Policy: "policy", HiddenColumns: []string{strings.Repeat("h", MaxNameBytes+1)}}}
		}, "limit_exceeded"},
		{"total cells", func(r *request) {
			r.Left.Rows = make([][]apicontract.TypedValue, MaxRowsPerSide)
			r.Right.Rows = make([][]apicontract.TypedValue, MaxRowsPerSide)
			r.Left.Columns = testColumns(MaxColumns)
			r.Right.Columns = testColumns(MaxColumns)
			for i := range r.Left.Rows {
				r.Left.Rows[i] = testRow(MaxColumns)
				r.Right.Rows[i] = testRow(MaxColumns)
			}
		}, "limit_exceeded"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var copy request
			encoded, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(encoded, &copy); err != nil {
				t.Fatal(err)
			}
			test.edit(&copy)
			if got := errorCode(validateRequestBounds(copy)); got != test.code {
				t.Fatalf("error code = %q, want %q", got, test.code)
			}
		})
	}
}

func TestCheckFieldsRejectsNonObjectsAndUnknownShape(t *testing.T) {
	if err := checkFields(nil, "recordset"); err == nil {
		t.Fatal("null object accepted")
	}
	if err := checkFields(map[string]any{}, "unsupported"); err == nil {
		t.Fatal("unsupported object shape accepted")
	}
	if err := checkFields(map[string]any{"columns": []any{}, "rows": []any{}, "extra": true}, "recordset"); err == nil {
		t.Fatal("unknown recordset field accepted")
	}
	if err := checkFields(map[string]any{"columns": []any{map[string]any{"name": "c", "type": "string", "extra": true}}, "rows": []any{}}, "recordset"); err == nil {
		t.Fatal("unknown nested column field accepted")
	}
	if err := checkFields(map[string]any{"columns": []any{}, "rows": []any{[]any{map[string]any{"type": "string"}}}}, "recordset"); err == nil {
		t.Fatal("missing nested typed value field accepted")
	}
	if err := checkFields(map[string]any{"rowCount": json.Number("0"), "limitations": []any{map[string]any{}}}, "sideInfo"); err == nil {
		t.Fatal("missing nested limitation fields accepted")
	}
	if err := checkFieldKind("request", "left", nil); err == nil {
		t.Fatal("null recordset accepted")
	}
	if err := checkFieldKind("recordset", "rows", nil); err == nil {
		t.Fatal("null rows accepted")
	}
	if err := checkFieldKind("sideInfo", "limitations", nil); err == nil {
		t.Fatal("null limitations accepted")
	}
	if err := checkFieldKind("limitation", "hiddenColumns", nil); err == nil {
		t.Fatal("null hiddenColumns accepted")
	}
	if err := checkFieldKind("request", "key", nil); err == nil {
		t.Fatal("null key accepted")
	}
	if err := checkTypedValueKind(map[string]any{"type": "integer", "value": nil}); err == nil {
		t.Fatal("null integer text accepted")
	}
	if err := checkTypedValueKind(map[string]any{"type": nil, "value": nil}); err == nil {
		t.Fatal("non-string discriminator accepted")
	}
	for _, item := range []struct {
		typeName string
		value    any
	}{
		{"string", "text"}, {"integer", "7"}, {"decimal", "1.00"},
		{"date", "2026-10-11"}, {"datetime", "2026-10-11T00:00:00Z"},
		{"number", json.Number("1.25")}, {"boolean", true}, {"null", nil},
	} {
		if err := checkTypedValueKind(map[string]any{"type": item.typeName, "value": item.value}); err != nil {
			t.Errorf("%s typed-value case rejected: %v", item.typeName, err)
		}
	}
	for _, value := range []map[string]any{
		{"type": "boolean", "value": "true"},
		{"type": "number", "value": nil},
		{"type": "unknown", "value": "x"},
	} {
		if err := checkTypedValueKind(value); err == nil {
			t.Errorf("invalid typed value accepted: %+v", value)
		}
	}
	if err := checkFieldKind("request", "abi", nil); err == nil {
		t.Fatal("null ABI accepted")
	}
	if err := checkFieldKind("request", "limit", nil); err == nil {
		t.Fatal("null limit accepted")
	}
	if err := checkFieldKind("sideInfo", "rowCount", nil); err == nil {
		t.Fatal("null rowCount accepted")
	}
	if err := checkFieldKind("limitation", "rowsFiltered", nil); err == nil {
		t.Fatal("null rowsFiltered accepted")
	}
	if err := checkFieldKind("unknown", "other", nil); err != nil {
		t.Fatalf("unconstrained optional field rejected: %v", err)
	}
	for _, field := range []struct {
		shape string
		key   string
		value any
	}{
		{"request", "right", map[string]any{}},
		{"request", "leftInfo", map[string]any{}},
		{"request", "rightInfo", map[string]any{}},
		{"request", "key", []any{}},
		{"request", "abi", "recordsetcompare/1"},
		{"request", "limit", json.Number("1")},
		{"recordset", "columns", []any{}},
		{"recordset", "rows", []any{}},
		{"sideInfo", "limitations", []any{}},
		{"sideInfo", "rowCount", json.Number("0")},
		{"limitation", "hiddenColumns", []any{}},
		{"limitation", "policy", "policy"},
		{"limitation", "rowsFiltered", true},
		{"typedValue", "value", "exact text"},
		{"typedValue", "type", "integer"},
	} {
		if err := checkFieldKind(field.shape, field.key, field.value); err != nil {
			t.Errorf("%s.%s rejected: %v", field.shape, field.key, err)
		}
	}
	for _, shape := range []string{"request", "recordset", "column", "typedValue", "sideInfo", "limitation", "unknown"} {
		if err := checkFields(map[string]any{}, shape); err == nil {
			t.Errorf("empty %s object accepted", shape)
		}
	}
	for _, shape := range []string{"request", "recordset", "column", "typedValue", "sideInfo", "limitation", "unknown"} {
		_ = requiredFields(shape)
	}
}

func testColumns(count int) []apicontract.Column {
	columns := make([]apicontract.Column, count)
	for i := range columns {
		columns[i] = apicontract.Column{Name: "c" + strconv.Itoa(i), Type: "string"}
	}
	return columns
}

func testRow(count int) []apicontract.TypedValue {
	row := make([]apicontract.TypedValue, count)
	for i := range row {
		row[i] = apicontract.NewStringValue("")
	}
	return row
}

func TestCheckFieldsRequiresCanonicalKnownShapes(t *testing.T) {
	if err := checkFields(map[string]any{"x": true}, "unsupported"); err == nil {
		t.Fatal("unknown shape accepted")
	}
	if err := checkFields(map[string]any{"ABI": "x"}, "request"); err == nil {
		t.Fatal("folded field accepted")
	}
	if err := checkFields(map[string]any{"abi": "x"}, "request"); err == nil {
		t.Fatal("missing required fields accepted")
	}
}

func errorCode(err error) string {
	var typed *bridgeError
	if !errors.As(err, &typed) {
		return ""
	}
	return typed.code
}

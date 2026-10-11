// Package bridge implements the bounded, receipt-free JSON ABI used by the
// linked WebAssembly entry point. It has no JavaScript Number conversion.
package bridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/recordsetcompare"
)

const (
	ABI                = "recordsetcompare/1"
	MaxInputBytes      = 1 << 20
	MaxOutputBytes     = 512 << 10
	MaxRowsPerSide     = 100
	MaxColumns         = 40
	MaxKeyColumns      = 40
	MaxNameBytes       = 256
	MaxCellBytes       = 16 << 10
	MaxJSONDepth       = 16
	MaxErrorMessage    = 256
	maxTotalCells      = 6_000
	maxDistributionCol = 256
)

type request struct {
	ABI                string                      `json:"abi"`
	Left               apicontract.Recordset       `json:"left"`
	Right              apicontract.Recordset       `json:"right"`
	LeftInfo           apicontract.CompareSideInfo `json:"leftInfo"`
	RightInfo          apicontract.CompareSideInfo `json:"rightInfo"`
	Key                []string                    `json:"key"`
	DistributionColumn string                      `json:"distributionColumn,omitempty"`
	Limit              int                         `json:"limit,omitempty"`
}

type response struct {
	ABI   string                 `json:"abi"`
	OK    bool                   `json:"ok"`
	Diff  *recordsetcompare.Diff `json:"diff,omitempty"`
	Error *wireError             `json:"error,omitempty"`
}

type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HandleJSON accepts and returns JSON strings so exact integer and decimal
// text never crosses a JavaScript numeric conversion.
func HandleJSON(input string) string {
	out, err := handle([]byte(input))
	if err == nil {
		return string(out)
	}
	return encodeError(err, MaxOutputBytes)
}

func encodeError(err error, outputLimit int) string {
	var typed *bridgeError
	if !errors.As(err, &typed) {
		typed = &bridgeError{code: "invalid_request", message: "request could not be processed"}
	}
	encoded, marshalErr := json.Marshal(response{
		ABI: ABI,
		OK:  false,
		Error: &wireError{
			Code:    typed.code,
			Message: boundedMessage(typed.message),
		},
	})
	if marshalErr != nil || len(encoded) > outputLimit {
		return `{"abi":"recordsetcompare/1","ok":false,"error":{"code":"internal_error","message":"response encoding failed"}}`
	}
	return string(encoded)
}

type bridgeError struct {
	code    string
	message string
}

func (e *bridgeError) Error() string { return e.code + ": " + e.message }

func fail(code, message string) error {
	return &bridgeError{code: code, message: message}
}

func boundedMessage(message string) string {
	if len(message) <= MaxErrorMessage {
		return message
	}
	return message[:MaxErrorMessage]
}

func handle(data []byte) ([]byte, error) {
	return handleWithEncoder(data, encodeSuccess)
}

func handleWithEncoder(data []byte, encode func(recordsetcompare.Diff) ([]byte, error)) ([]byte, error) {
	if len(data) > MaxInputBytes {
		return nil, fail("input_too_large", "request exceeds the 1 MiB input limit")
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fail("invalid_request", "request root must be a JSON object")
	}
	if err := checkJSONDepth(data); err != nil {
		return nil, fail("invalid_json", err.Error())
	}
	var req request
	if err := apicontract.DecodeStrict(data, &req); err != nil {
		return nil, fail("invalid_request", err.Error())
	}
	// checkJSONDepth has already validated the complete JSON document, so this
	// second decode can only fail if the standard decoder behavior diverges.
	raw, _ := decodeRawJSON(data)
	if err := checkExactRequestFields(raw); err != nil {
		return nil, fail("invalid_request", err.Error())
	}
	if req.ABI != ABI {
		return nil, fail("invalid_abi", "unsupported ABI")
	}
	if err := validateRequestBounds(req); err != nil {
		return nil, err
	}
	diff, err := recordsetcompare.CompareData(req.Left, req.Right, req.LeftInfo, req.RightInfo, recordsetcompare.Options{
		Key: req.Key, DistributionColumn: req.DistributionColumn, Limit: req.Limit,
	})
	if err != nil {
		return nil, fail("compare_failed", err.Error())
	}
	encoded, err := encode(diff)
	if err != nil {
		return nil, fail("response_invalid", err.Error())
	}
	if len(encoded) > MaxOutputBytes {
		return nil, fail("output_too_large", "complete diff exceeds the 512 KiB output limit")
	}
	return encoded, nil
}

func decodeRawJSON(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func encodeSuccess(diff recordsetcompare.Diff) ([]byte, error) {
	return json.Marshal(response{ABI: ABI, OK: true, Diff: &diff})
}

func validateRequestBounds(req request) error {
	if req.ABI == "" {
		return fail("invalid_request", "abi is required")
	}
	if len(req.Key) == 0 || len(req.Key) > MaxKeyColumns {
		return fail("invalid_request", "key must contain between 1 and 40 columns")
	}
	for _, name := range req.Key {
		if len(name) == 0 || len(name) > MaxNameBytes {
			return fail("invalid_request", "key column name exceeds the 256 byte limit")
		}
	}
	if len(req.DistributionColumn) > maxDistributionCol {
		return fail("invalid_request", "distribution column name exceeds the 256 byte limit")
	}
	if req.Limit < 0 || req.Limit > MaxRowsPerSide {
		return fail("limit_exceeded", "diff output limit must be between 1 and 100")
	}
	if err := validateRecordsetBounds("left", req.Left); err != nil {
		return err
	}
	if err := validateRecordsetBounds("right", req.Right); err != nil {
		return err
	}
	if len(req.Left.Rows)*len(req.Left.Columns)+len(req.Right.Rows)*len(req.Right.Columns) > maxTotalCells {
		return fail("limit_exceeded", "total cell count exceeds the bridge limit")
	}
	if err := validateSideInfoBounds(req.LeftInfo); err != nil {
		return err
	}
	if err := validateSideInfoBounds(req.RightInfo); err != nil {
		return err
	}
	return nil
}

func validateRecordsetBounds(side string, recordset apicontract.Recordset) error {
	if len(recordset.Rows) > MaxRowsPerSide {
		return fail("limit_exceeded", side+" recordset exceeds 100 rows")
	}
	if len(recordset.Columns) > MaxColumns {
		return fail("limit_exceeded", side+" recordset exceeds 40 columns")
	}
	for _, column := range recordset.Columns {
		if len(column.Name) == 0 || len(column.Name) > MaxNameBytes || len(column.Type) > 32 {
			return fail("limit_exceeded", side+" column metadata exceeds its byte limit")
		}
	}
	for _, row := range recordset.Rows {
		for _, value := range row {
			if len(value.Str) > MaxCellBytes {
				return fail("limit_exceeded", side+" cell text exceeds the 16 KiB limit")
			}
		}
	}
	return nil
}

func validateSideInfoBounds(info apicontract.CompareSideInfo) error {
	for _, limitation := range info.Limitations {
		if len(limitation.Policy) > MaxNameBytes {
			return fail("limit_exceeded", "limitation policy exceeds the 256 byte limit")
		}
		if len(limitation.HiddenColumns) > MaxColumns {
			return fail("limit_exceeded", "hidden column list exceeds 40 columns")
		}
		for _, name := range limitation.HiddenColumns {
			if len(name) == 0 || len(name) > MaxNameBytes {
				return fail("limit_exceeded", "hidden column name exceeds the 256 byte limit")
			}
		}
	}
	return nil
}

func checkJSONDepth(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, depth int) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	depth++
	if depth > MaxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d levels", MaxJSONDepth)
	}
	switch delim {
	case '{':
		for decoder.More() {
			if _, err := decoder.Token(); err != nil {
				return err
			}
			if err := scanJSONValue(decoder, depth); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder, depth); err != nil {
				return err
			}
		}
		_, err = decoder.Token()
	}
	return err
}

func checkExactRequestFields(root any) error {
	return checkFields(root, "request")
}

func checkFields(value any, shape string) error {
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s must be a JSON object", shape)
	}
	var allowed map[string]bool
	switch shape {
	case "request":
		allowed = fieldSet("abi", "left", "right", "leftInfo", "rightInfo", "key", "distributionColumn", "limit")
	case "recordset":
		allowed = fieldSet("columns", "rows")
	case "column":
		allowed = fieldSet("name", "type")
	case "typedValue":
		allowed = fieldSet("type", "value")
	case "sideInfo":
		allowed = fieldSet("rowCount", "limitations")
	case "limitation":
		allowed = fieldSet("policy", "rowsFiltered", "hiddenColumns")
	default:
		return fmt.Errorf("internal bridge shape %q is unsupported", shape)
	}
	for key, child := range object {
		if !allowed[key] {
			return fmt.Errorf("field %q does not use its canonical spelling", key)
		}
		if err := checkFieldKind(shape, key, child); err != nil {
			return err
		}
		var childShape string
		switch shape {
		case "request":
			switch key {
			case "left", "right":
				childShape = "recordset"
			case "leftInfo", "rightInfo":
				childShape = "sideInfo"
			}
		case "recordset":
			switch key {
			case "columns":
				values := child.([]any)
				for _, item := range values {
					if err := checkFields(item, "column"); err != nil {
						return err
					}
				}
			case "rows":
				rows := child.([]any)
				for _, row := range rows {
					cells, ok := row.([]any)
					if !ok {
						return fmt.Errorf("recordset row must be a JSON array")
					}
					for _, cell := range cells {
						if err := checkFields(cell, "typedValue"); err != nil {
							return err
						}
					}
				}
			}
		case "sideInfo":
			if key == "limitations" {
				values := child.([]any)
				for _, item := range values {
					if err := checkFields(item, "limitation"); err != nil {
						return err
					}
				}
			}
		}
		if childShape != "" {
			if err := checkFields(child, childShape); err != nil {
				return err
			}
		}
	}
	for _, required := range requiredFields(shape) {
		if _, ok := object[required]; !ok {
			return fmt.Errorf("field %q is required", required)
		}
	}
	if shape == "typedValue" {
		if err := checkTypedValueKind(object); err != nil {
			return err
		}
	}
	return nil
}

func checkTypedValueKind(object map[string]any) error {
	typeName, ok := object["type"].(string)
	if !ok {
		return fmt.Errorf("field %q must be a JSON string", "type")
	}
	value := object["value"]
	valid := false
	switch typeName {
	case "string", "integer", "decimal", "date", "datetime":
		_, valid = value.(string)
	case "number":
		_, valid = value.(json.Number)
	case "boolean":
		_, valid = value.(bool)
	case "null":
		valid = value == nil
	default:
		return fmt.Errorf("field %q has an unsupported typed-value discriminator", "type")
	}
	if !valid {
		return fmt.Errorf("typed value field %q does not match its discriminator", "value")
	}
	return nil
}

func checkFieldKind(shape, key string, value any) error {
	want := ""
	switch shape {
	case "request":
		switch key {
		case "left", "right", "leftInfo", "rightInfo":
			if _, ok := value.(map[string]any); !ok {
				return fmt.Errorf("field %q must be a JSON object", key)
			}
		case "key":
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("field %q must be a JSON array", key)
			}
		case "abi", "distributionColumn":
			want = "string"
		case "limit":
			want = "number"
		}
	case "recordset":
		if key == "columns" || key == "rows" {
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("field %q must be a JSON array", key)
			}
		}
	case "sideInfo":
		if key == "rowCount" {
			want = "number"
		}
		if key == "limitations" {
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("field %q must be a JSON array", key)
			}
		}
	case "limitation":
		if key == "policy" {
			want = "string"
		}
		if key == "rowsFiltered" {
			want = "boolean"
		}
		if key == "hiddenColumns" {
			if _, ok := value.([]any); !ok {
				return fmt.Errorf("field %q must be a JSON array", key)
			}
		}
	case "typedValue":
		if key == "type" {
			want = "string"
		}
	}
	if want != "" {
		valid := false
		switch want {
		case "string":
			_, valid = value.(string)
		case "number":
			_, valid = value.(json.Number)
		case "boolean":
			_, valid = value.(bool)
		}
		if !valid {
			return fmt.Errorf("field %q must be a JSON %s", key, want)
		}
	}
	return nil
}

func requiredFields(shape string) []string {
	switch shape {
	case "request":
		return []string{"abi", "left", "right", "leftInfo", "rightInfo", "key"}
	case "recordset":
		return []string{"columns", "rows"}
	case "column":
		return []string{"name", "type"}
	case "typedValue":
		return []string{"type", "value"}
	case "sideInfo":
		return []string{"rowCount", "limitations"}
	case "limitation":
		return []string{"policy", "rowsFiltered", "hiddenColumns"}
	default:
		return nil
	}
}

func fieldSet(names ...string) map[string]bool {
	fields := make(map[string]bool, len(names))
	for _, name := range names {
		fields[name] = true
	}
	return fields
}

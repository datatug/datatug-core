package dataaccess

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
)

func tableColumns(rs recordset.Recordset) ([]string, error) {
	if isNilValue(rs) || rs.ColumnsCount() < 1 || rs.ColumnsCount() > 128 {
		return nil, fmt.Errorf("dataaccess: unsupported table columns")
	}
	names := make([]string, rs.ColumnsCount())
	seen := make(map[string]bool)
	for i := range names {
		column := rs.GetColumnByIndex(i)
		if isNilValue(column) || !canonical(column.Name()) || seen[column.Name()] {
			return nil, fmt.Errorf("dataaccess: ambiguous table column")
		}
		names[i], seen[column.Name()] = column.Name(), true
	}
	return names, nil
}

func tableCell(value any, maxBytes uint32) ([]byte, error) {
	switch v := value.(type) {
	case nil, bool, int64:
		// These concrete built-in scalar types cannot invoke custom marshaling.
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("dataaccess: nonfinite table value")
		}
	case string:
		if len(v) > int(maxBytes) || !utf8.ValidString(v) {
			return nil, fmt.Errorf("dataaccess: invalid table text")
		}
	case []byte:
		if len(v) > int(maxBytes) || base64.StdEncoding.EncodedLen(len(v))+13 > int(maxBytes) {
			return nil, fmt.Errorf("dataaccess: table value exceeds byte limit")
		}
		encoded := append([]byte(`{"base64":"`), make([]byte, base64.StdEncoding.EncodedLen(len(v)))...)
		base64.StdEncoding.Encode(encoded[11:], v)
		return append(encoded, '"', '}'), nil
	default:
		return nil, fmt.Errorf("dataaccess: unsupported table value")
	}
	// All admitted values are finite built-in scalars; encoding cannot fail.
	data, _ := json.Marshal(value)
	if len(data) > int(maxBytes) {
		clear(data)
		return nil, fmt.Errorf("dataaccess: table value exceeds byte limit")
	}
	return data, nil
}

func (r *Result) appendTable(data []byte, maxBytes uint32) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.payload == nil {
		return fmt.Errorf("dataaccess: result is closed")
	}
	if len(data) > int(maxBytes)-len(r.payload) {
		return fmt.Errorf("dataaccess: table preview exceeds byte limit")
	}
	r.payload = append(r.payload, data...)
	return nil
}

func (r *Result) readTableRows(ctx context.Context, reader dal.RecordsetReader, op TableRead) error {
	names, err := tableColumns(reader.Recordset())
	if err != nil {
		return err
	}
	columns, _ := json.Marshal(names) // bounded canonical built-in strings only
	err = r.appendTable([]byte(`{"columns":`), op.MaxBytes)
	if err == nil {
		err = r.appendTable(columns, op.MaxBytes)
	}
	clear(columns) // no serialized metadata scratch survives fresh resolver I/O
	if err != nil {
		return err
	}
	if err := r.appendTable([]byte(`,"rows":[`), op.MaxBytes); err != nil {
		return err
	}
	var count uint32
	hasMore := false
	for {
		if err := r.guard(ctx); err != nil {
			return err
		}
		row, rs, readErr := reader.Next()
		if err := r.guard(ctx); err != nil {
			return err
		}
		if errors.Is(readErr, dal.ErrNoMoreRecords) {
			break
		}
		if readErr != nil || isNilValue(row) {
			return r.authority.denial(DataRead, "TABLE_READ_FAILED")
		}
		current, err := tableColumns(rs)
		if err != nil || !equalTableColumns(names, current) {
			return fmt.Errorf("dataaccess: table columns changed")
		}
		if count == op.MaxRows {
			hasMore = true
			break
		}
		values, err := row.Data(rs)
		if err != nil || len(values) != len(names) {
			return r.authority.denial(DataRead, "TABLE_VALUE_UNSUPPORTED")
		}
		if count != 0 {
			if err := r.appendTable([]byte(","), op.MaxBytes); err != nil {
				return err
			}
		}
		if err := r.appendTable([]byte("["), op.MaxBytes); err != nil {
			return err
		}
		for i, value := range values {
			if i != 0 {
				if err := r.appendTable([]byte(","), op.MaxBytes); err != nil {
					return err
				}
			}
			cell, err := tableCell(value, op.MaxBytes)
			if err != nil {
				return err
			}
			err = r.appendTable(cell, op.MaxBytes)
			clear(cell)
			if err != nil {
				return err
			}
		}
		if err := r.appendTable([]byte("]"), op.MaxBytes); err != nil {
			return err
		}
		count++
	}
	return r.appendTable([]byte(fmt.Sprintf(`],"preview":{"returnedRows":%d,"hasMore":%t,"complete":%t}}`, count, hasMore, !hasMore)), op.MaxBytes)
}

func equalTableColumns(a, b []string) bool { return strings.Join(a, "\x00") == strings.Join(b, "\x00") }

func validateTableCell(data json.RawMessage, maxBytes uint32) error {
	var scalar any
	if err := tableDecode(data, &scalar); err != nil {
		return err
	}
	if value, ok := scalar.(json.Number); ok {
		if !strings.ContainsAny(value.String(), ".eE") {
			if _, err := strconv.ParseInt(value.String(), 10, 64); err == nil {
				return nil
			}
		}
		// JSON syntax was checked above. Out-of-range conversion yields an
		// infinity, rejected by the same closed scalar encoder as native cells.
		number, _ := value.Float64()
		encoded, err := tableCell(number, maxBytes)
		defer clear(encoded)
		if err != nil || !bytes.Equal(encoded, bytes.TrimSpace(data)) {
			return fmt.Errorf("dataaccess: noncanonical table number")
		}
		return nil
	}
	if len(data) > 0 && bytes.TrimSpace(data)[0] == '{' {
		var blob struct {
			Base64 *string `json:"base64"`
		}
		if err := tableDecode(data, &blob); err != nil || blob.Base64 == nil {
			return fmt.Errorf("dataaccess: unsupported table blob")
		}
		decoded, err := base64.StdEncoding.Strict().DecodeString(*blob.Base64)
		defer clear(decoded)
		if err != nil {
			return err
		}
		canonical, err := tableCell(decoded, maxBytes)
		defer clear(canonical)
		if err != nil || !bytes.Equal(canonical, bytes.TrimSpace(data)) {
			return fmt.Errorf("dataaccess: noncanonical table blob")
		}
		return nil
	}
	encoded, err := tableCell(scalar, maxBytes)
	defer clear(encoded)
	if err == nil && !bytes.Equal(encoded, bytes.TrimSpace(data)) {
		return fmt.Errorf("dataaccess: noncanonical table scalar")
	}
	return err
}

func validateTablePayload(data []byte, operation TableRead) error {
	if len(data) > int(operation.MaxBytes) {
		return fmt.Errorf("dataaccess: table preview exceeds byte limit")
	}
	var payload struct {
		Columns []string            `json:"columns"`
		Rows    [][]json.RawMessage `json:"rows"`
		Preview *struct {
			ReturnedRows *uint32 `json:"returnedRows"`
			HasMore      *bool   `json:"hasMore"`
			Complete     *bool   `json:"complete"`
		} `json:"preview"`
	}
	err := tableDecode(data, &payload)
	defer func() {
		for _, row := range payload.Rows {
			for _, cell := range row {
				clear(cell)
			}
		}
	}()
	if err != nil {
		return err
	}
	p := payload.Preview
	if len(payload.Columns) == 0 || len(payload.Columns) > 128 || payload.Rows == nil || p == nil || p.ReturnedRows == nil || p.HasMore == nil || p.Complete == nil || int(*p.ReturnedRows) != len(payload.Rows) || *p.ReturnedRows > operation.MaxRows || *p.HasMore == *p.Complete || (*p.HasMore && *p.ReturnedRows != operation.MaxRows) {
		return fmt.Errorf("dataaccess: invalid table preview metadata")
	}
	seen := make(map[string]bool)
	for _, name := range payload.Columns {
		if !canonical(name) || seen[name] {
			return fmt.Errorf("dataaccess: ambiguous table column")
		}
		seen[name] = true
	}
	for _, row := range payload.Rows {
		if len(row) != len(payload.Columns) {
			return fmt.Errorf("dataaccess: invalid table row width")
		}
		for _, cell := range row {
			if err := validateTableCell(cell, operation.MaxBytes); err != nil {
				return err
			}
		}
	}
	return nil
}

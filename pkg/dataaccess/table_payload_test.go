package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/require"
)

func TestTableCellClosedScalarAndBlobFidelity(t *testing.T) {
	for _, v := range []any{nil, true, false, int64(math.MaxInt64), int64(math.MinInt64), 1.25, float64(1e20), "\x00<&é", []byte(nil), []byte{}, []byte{0, 255}} {
		encoded, err := tableCell(v, 4096)
		require.NoError(t, err)
		require.NoError(t, validateTableCell(encoded, 4096), string(encoded))
		clear(encoded)
	}
	for _, v := range []any{int(1), json.Number("1"), struct{}{}, math.NaN(), math.Inf(1), string([]byte{255}), strings.Repeat("x", 5), []byte{1, 2, 3}} {
		_, err := tableCell(v, 4)
		require.Error(t, err)
	}
	_, err := tableCell("\x00", 3)
	require.Error(t, err, "escaped serialization, not raw byte length, must fit")
	for _, raw := range []string{`9223372036854775808`, `1e400`, `1.0`, `"\ud800"`, `"\u0041"`, `[]`, `{}`, `{"base64":null}`, `{"base64":"!!!"}`, `{"base64":"AA==\n"}`, `{"Base64":""}`, `{"base64":"","BASE64":"AA=="}`, `{"other":""}`, `1 2`, "\xff"} {
		require.Error(t, validateTableCell([]byte(raw), 4096), raw)
	}
	require.Error(t, validateTableCell([]byte(`{"base64":"AAAA"}`), 12))
}

type previewColumnCount struct {
	recordset.Recordset
	count int
}

func (r previewColumnCount) ColumnsCount() int { return r.count }
func TestTableColumnIdentityAndRecordsetAdmission(t *testing.T) {
	for _, rs := range []recordset.Recordset{nil, previewColumnCount{count: -1}, previewColumnCount{count: 129}, recordset.NewColumnarRecordset("empty"), recordset.NewColumnarRecordset("nil", nil), previewRecordset(t, []string{"a", "a"}), previewRecordset(t, []string{" "})} {
		_, err := tableColumns(rs)
		require.Error(t, err)
	}
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	span := newPreviewSpan(&previewAdapter{reader: &previewReader{}})
	_, err := a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), previewSource(span))
	require.Error(t, err)
}

func TestTablePayloadRejectsTamperedCountCompletenessWidthAndTypes(t *testing.T) {
	valid := `{"columns":["x"],"rows":[[1]],"preview":{"returnedRows":1,"hasMore":false,"complete":true}}`
	require.NoError(t, validateTablePayload([]byte(valid), previewOperation()))
	for _, raw := range []string{
		`null`, `{}`, `{"columns":["x"],"rows":[],"preview":null}`,
		`{"columns":["x"],"rows":[],"preview":{"returnedRows":0,"complete":true}}`,
		strings.Replace(valid, `"returnedRows":1`, `"returnedRows":2`, 1),
		strings.Replace(valid, `"complete":true`, `"complete":false`, 1),
		strings.Replace(valid, `"hasMore":false`, `"hasMore":true,"complete":false`, 1),
		strings.Replace(valid, `"rows":[[1]]`, `"rows":[[]]`, 1),
		strings.Replace(valid, `"rows":[[1]]`, `"rows":[[{}]]`, 1),
		strings.Replace(valid, `"columns":["x"]`, `"columns":[" "]`, 1),
		strings.Replace(valid, `"columns":["x"]`, `"columns":["x","x"]`, 1),
		strings.Replace(valid, `"preview":`, `"Preview":{},"preview":`, 1),
		strings.Replace(valid, `"rows":[[1]]`, `"rows":[[1],[2],[3]]`, 1),
	} {
		require.Error(t, validateTablePayload([]byte(raw), previewOperation()), raw)
	}
	op := previewOperation()
	op.MaxBytes = 1
	require.Error(t, validateTablePayload([]byte(valid), op))
}

func TestTablePreviewEveryOutputByteLimitIsEnforcedBeforePublication(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	read := func(limit uint32) (*Result, error) {
		reader := &previewReader{rs: previewRecordset(t, []string{"id", "name"}, []any{int64(1), "Ada"}, []any{int64(2), "Grace"})}
		op := previewOperation()
		op.MaxBytes = limit
		return a.ReadTableResult(context.Background(), resultRequest(), op, previewSource(newPreviewSpan(&previewAdapter{reader: reader})))
	}
	baseline, err := read(4096)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, baseline.WritePayload(context.Background(), &out))
	baseline.Close()
	for limit := 1; limit < out.Len(); limit++ {
		r, err := read(uint32(limit))
		require.Error(t, err, "limit %d", limit)
		require.Nil(t, r)
	}
	exact, err := read(uint32(out.Len()))
	require.NoError(t, err)
	defer exact.Close()
}

func TestTablePrivateBufferCannotReappearAfterConcurrentPurge(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	_, request, _ := previewOperation().query()
	_, bound, cancel, err := a.prepareRequest(context.Background(), nil, request)
	require.NoError(t, err)
	defer cancel()
	r := a.newTableResult(context.Background(), resultRequest(), previewOperation(), bound, make([]byte, 0, 128))
	r.Close()
	require.Error(t, r.appendTable([]byte("secret"), 128))
	require.Error(t, r.sealTable())
	require.False(t, r.ownCleanup(func() {}))
	require.Error(t, r.readTableRows(context.Background(), &previewReader{rs: previewRecordset(t, []string{"id"})}, previewOperation()))
}

func TestTablePreviewCancellationAtReaderOpenAndClose(t *testing.T) {
	for _, atOpen := range []bool{false, true} {
		t.Run(map[bool]string{false: "close", true: "open"}[atOpen], func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &previewReader{rs: previewRecordset(t, []string{"id"})}
			session := previewFunctionAdapter{execute: func(dal.Query) (dal.RecordsetReader, error) {
				if atOpen {
					cancel()
				}
				return reader, nil
			}}
			if !atOpen {
				reader.closeHook = func() error { cancel(); return nil }
			}
			a := testAuthority(t, fixedResolver(qualifiedResolution()))
			r, err := a.ReadTableResult(ctx, resultRequest(), previewOperation(), previewSource(newPreviewSpan(session)))
			require.Error(t, err)
			require.Nil(t, r)
		})
	}
}

func TestTablePreviewCancellationBeforeFirstNativeNext(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	_, request, _ := previewOperation().query()
	_, bound, cancel, err := a.prepareRequest(context.Background(), nil, request)
	require.NoError(t, err)
	defer cancel()
	r := a.newTableResult(context.Background(), resultRequest(), previewOperation(), bound, make([]byte, 0, 4096))
	defer r.Close()
	ctx, stop := context.WithCancel(context.Background())
	stop()
	reader := &previewReader{rs: previewRecordset(t, []string{"id"})}
	require.Error(t, r.readTableRows(ctx, reader, previewOperation()))
	require.Zero(t, reader.index)
}

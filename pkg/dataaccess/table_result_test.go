package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func previewEnvelope(t *testing.T) (TableResultEnvelope, time.Time, Resolution) {
	t.Helper()
	started := time.Now()
	resolution := qualifiedResolution()
	a := testAuthority(t, fixedResolver(resolution))
	r := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}))
	var out bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &out))
	env, err := DecodeTableResultEnvelope(out.Bytes())
	require.NoError(t, err)
	return env, started, resolution
}
func TestTableResultStrictClosedVersionedEnvelope(t *testing.T) {
	env, _, _ := previewEnvelope(t)
	data, err := json.Marshal(env)
	require.NoError(t, err)
	for _, input := range [][]byte{bytes.Replace(data, []byte(`"version":2`), []byte(`"version":2,"VERSION":2`), 1), bytes.Replace(data, []byte(`"realm":"sneat"`), []byte(`"realm":"sneat","REALM":"sneat"`), 1), []byte(strings.Repeat("x", 294913)), []byte(`{`), []byte(`null`), []byte(`{}`), []byte(`{"payload":{},"lease":{"version":1}}`), append(bytes.Clone(data), []byte(` {}`)...), bytes.Replace(data, []byte(`"profile":`), []byte(`"Profile":"bad","profile":`), 1), bytes.Replace(data, []byte(`"maxRows":2`), []byte(`"maxRows":2,"MAXROWS":1`), 1), bytes.Replace(data, []byte(`"complete":true`), []byte(`"complete":true,"COMPLETE":false`), 1), bytes.Replace(data, []byte(`"returnedRows":1`), []byte(`"returnedRows":2`), 1), bytes.Replace(data, []byte(`"profile":`), []byte(`"unknown":`), 1), []byte{255}} {
		_, err := DecodeTableResultEnvelope(input)
		require.Error(t, err, string(input))
	}
	var value any
	require.Error(t, tableDecode([]byte(`1 2`), &value))
	require.Error(t, tableDecode([]byte(`{"x":1,"x":2}`), &value))
	require.Error(t, tableDecode([]byte(`{`), &value))
}

func TestTableResultImportIndependentExpectedBindings(t *testing.T) {
	env, started, resolution := previewEnvelope(t)
	original := bytes.Clone(env.Payload)
	defer clear(original)
	for name, mutate := range map[string]func(*TableResultEnvelope){
		"version": func(e *TableResultEnvelope) { e.Lease.Version = 1 }, "profile": func(e *TableResultEnvelope) { e.Lease.Profile = "unknown" }, "object": func(e *TableResultEnvelope) { e.Lease.Operation.Object.Name = "Other" }, "namespace": func(e *TableResultEnvelope) { e.Lease.Operation.Object.Namespace = "other" }, "limits": func(e *TableResultEnvelope) { e.Lease.Operation.MaxRows++ },
		"subject": func(e *TableResultEnvelope) { e.Lease.Subject.ID = "other" }, "realm": func(e *TableResultEnvelope) { e.Lease.Subject.Realm = "other" }, "kind": func(e *TableResultEnvelope) { e.Lease.Subject.Kind = Agent }, "space": func(e *TableResultEnvelope) { e.Lease.Connection.Space = "other" }, "connection": func(e *TableResultEnvelope) { e.Lease.Connection.ID = "other" }, "source": func(e *TableResultEnvelope) { e.Lease.Connection.Fingerprint.EngineIdentity = "other" },
		"nonce": func(e *TableResultEnvelope) { e.Lease.Nonce = "other" }, "request": func(e *TableResultEnvelope) { e.Lease.RequestSHA256 = hashBytes([]byte("other")) }, "digest": func(e *TableResultEnvelope) { e.Lease.GrantSetDigest = "bad" }, "hash-shape": func(e *TableResultEnvelope) { e.Lease.PayloadSHA256 = "bad" }, "hash-mismatch": func(e *TableResultEnvelope) { e.Lease.PayloadSHA256 = hashBytes([]byte("other")) }, "time": func(e *TableResultEnvelope) { e.Lease.ServerTime = time.Time{} }, "interval-zero": func(e *TableResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime }, "interval-long": func(e *TableResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime.Add(61 * time.Second) },
		"malformed": func(e *TableResultEnvelope) { e.Payload = []byte(`not json`) }, "payload": func(e *TableResultEnvelope) { e.Payload = []byte(`{}`); e.Lease.PayloadSHA256 = hashBytes(e.Payload) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := env
			copy.Payload = bytes.Clone(original)
			mutate(&copy)
			a := testAuthority(t, fixedResolver(resolution))
			r, err := a.ImportTableResult(context.Background(), resultRequest(), started, previewOperation(), copy)
			require.Error(t, err)
			require.Nil(t, r)
			require.NotContains(t, string(copy.Payload), "columns")
		})
	}
	a := testAuthority(t, fixedResolver(resolution))
	for _, mode := range []string{"authority", "context", "request", "operation", "started", "future", "expired", "revoke"} {
		t.Run(mode, func(t *testing.T) {
			authority := a
			ctx := context.Background()
			request := resultRequest()
			op := previewOperation()
			start := started
			copy := env
			copy.Payload = bytes.Clone(original)
			switch mode {
			case "authority":
				authority = nil
			case "context":
				ctx = nil
			case "request":
				request = ResultRequest{}
			case "operation":
				op = TableRead{}
			case "started":
				start = time.Time{}
			case "future":
				start = time.Now().Add(time.Hour)
			case "expired":
				start = time.Now().Add(-time.Minute)
			case "revoke":
				resolution := qualifiedResolution()
				resolution.Grants = nil
				authority = testAuthority(t, fixedResolver(resolution))
			}
			r, err := authority.ImportTableResult(ctx, request, start, op, copy)
			require.Error(t, err)
			require.Nil(t, r)
		})
	}
}

func TestTableResultOriginalRequestAndCredentialCeilings(t *testing.T) {
	env, _, resolution := previewEnvelope(t)
	a := testAuthority(t, fixedResolver(resolution))
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	started := time.Now().Add(-time.Second)
	r, err := a.ImportTableResult(ctx, resultRequest(), started, previewOperation(), env)
	require.NoError(t, err)
	defer r.Close()
	deadline, _ := ctx.Deadline()
	require.False(t, r.authorization.deadline.After(deadline))
	<-ctx.Done()
	var out bytes.Buffer
	require.Error(t, r.WritePayload(context.Background(), &out))
	require.Zero(t, out.Len())
	assertResultPurged(t, r)
}

func TestTableResultEscapedUnicodeRejectedAcrossWireFields(t *testing.T) {
	env, _, _ := previewEnvelope(t)
	data, err := json.Marshal(env)
	require.NoError(t, err)
	for _, unit := range []string{`\ud800`, `\udc00`, `\ud800x`} {
		for name, pair := range map[string][2]string{
			"operation-name":      {`"name":"Customer"`, `"name":"` + unit + `"`},
			"operation-namespace": {`"namespace":"sales"`, `"namespace":"` + unit + `"`},
			"column":              {`"columns":["id"]`, `"columns":["` + unit + `"]`},
			"cell":                {`"rows":[[1]]`, `"rows":[["` + unit + `"]]`},
		} {
			t.Run(name+unit, func(t *testing.T) {
				changed := bytes.Replace(data, []byte(pair[0]), []byte(pair[1]), 1)
				require.NotEqual(t, data, changed)
				_, err := DecodeTableResultEnvelope(changed)
				require.Error(t, err)
			})
		}
	}
	for _, pair := range [][2]string{{`"name":"Customer"`, `"name":"\ud834\udd1e"`}, {`"columns":["id"]`, `"columns":["\ud834\udd1e"]`}} {
		valid := bytes.Replace(data, []byte(pair[0]), []byte(pair[1]), 1)
		_, err := DecodeTableResultEnvelope(valid)
		require.NoError(t, err, "valid escaped surrogate pairs remain valid Unicode")
	}
}

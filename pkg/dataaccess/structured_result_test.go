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

func structuredEnvelope(t *testing.T) (StructuredResultEnvelope, time.Time, Resolution) {
	t.Helper()
	resolution := qualifiedResolution()
	a := testAuthority(t, fixedResolver(resolution))
	op := structuredOperation()
	started := time.Now()
	r := structuredResult(t, a, context.Background(), op, structuredSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id", "text"}, []any{int64(9223372036854775807), "private value"})}}))
	var out bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &out))
	envelope, err := DecodeStructuredResultEnvelope(out.Bytes())
	require.NoError(t, err)
	_, err = DecodeResultEnvelope(out.Bytes())
	require.Error(t, err)
	_, err = DecodeTableResultEnvelope(out.Bytes())
	require.Error(t, err)
	return envelope, started, resolution
}

func TestStructuredResultVersion3TransportAndIndependentBindings(t *testing.T) {
	envelope, started, resolution := structuredEnvelope(t)
	owned := envelope.Payload
	a := testAuthority(t, fixedResolver(resolution))
	op := structuredOperation()
	request := structuredRequest(t, op)
	r, err := a.ImportStructuredResult(context.Background(), request, started, op, envelope)
	require.NoError(t, err)
	defer r.Close()
	require.EqualValues(t, make([]byte, len(owned)), owned)
	var out bytes.Buffer
	require.NoError(t, r.WritePayload(context.Background(), &out))
	require.Contains(t, out.String(), `9223372036854775807`)
	require.Contains(t, out.String(), "private value")
	require.Equal(t, 3, r.metadata.Version)
	var wire bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &wire))
	decoded, err := DecodeStructuredResultEnvelope(wire.Bytes())
	require.NoError(t, err)
	require.Equal(t, op, decoded.Lease.Operation)
	require.NotEmpty(t, decoded.Lease.ExecutedTugQL)
	for _, mutate := range []func(*StructuredResultEnvelope){
		func(e *StructuredResultEnvelope) { e.Lease.Version = 2 },
		func(e *StructuredResultEnvelope) { e.Lease.Profile = TableReadProfile },
		func(e *StructuredResultEnvelope) {
			e.Lease.Operation.Parameters = `{"x":{"type":"integer","value":"1"}}`
		},
		func(e *StructuredResultEnvelope) { e.Lease.Operation.MaxRows++ },
		func(e *StructuredResultEnvelope) { e.Lease.Subject.ID = "other" },
		func(e *StructuredResultEnvelope) { e.Lease.Connection.Space = "other" },
		func(e *StructuredResultEnvelope) { e.Lease.Connection.LocalAuthorityID = "local-forgery" },
		func(e *StructuredResultEnvelope) { e.Lease.Nonce = "other-fresh-nonce" },
		func(e *StructuredResultEnvelope) { e.Lease.RequestSHA256 = hashBytes([]byte("other")) },
		func(e *StructuredResultEnvelope) { e.Lease.PayloadSHA256 = strings.Repeat("0", 64) },
		func(e *StructuredResultEnvelope) { e.Lease.GrantSetDigest = "invalid" },
		func(e *StructuredResultEnvelope) { e.Lease.ServerTime = time.Time{} },
		func(e *StructuredResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime },
		func(e *StructuredResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime.Add(time.Hour) },
		func(e *StructuredResultEnvelope) { e.Lease.ExecutionSHA256 = "invalid" },
		func(e *StructuredResultEnvelope) {
			e.Lease.ExecutedTugQL = "bad: query"
			e.Lease.ExecutionSHA256 = hashBytes([]byte(e.Lease.ExecutedTugQL))
		},
		func(e *StructuredResultEnvelope) {
			e.Payload = []byte(`[]`)
			e.Lease.PayloadSHA256 = hashBytes(e.Payload)
		},
		func(e *StructuredResultEnvelope) {
			e.Payload = []byte(`{}`)
			e.Lease.PayloadSHA256 = hashBytes(e.Payload)
		},
	} {
		copy := decoded
		copy.Payload = bytes.Clone(decoded.Payload)
		mutate(&copy)
		owned := copy.Payload
		_, err := a.ImportStructuredResult(context.Background(), request, started, op, copy)
		require.Error(t, err)
		require.EqualValues(t, make([]byte, len(owned)), owned)
	}
}

func TestStructuredResultDecodeStrictnessAndImportAdmission(t *testing.T) {
	envelope, started, resolution := structuredEnvelope(t)
	op := structuredOperation()
	request := structuredRequest(t, op)
	a := testAuthority(t, fixedResolver(resolution))
	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	for _, data := range [][]byte{nil, []byte(`{}`), []byte(`{"payload":{},"payload":{},"lease":{}}`), []byte(`{"payload":{},"lease":{},"extra":null}`), append(bytes.Clone(encoded), []byte(` {}`)...), bytes.Repeat([]byte(" "), 2359297)} {
		_, err := DecodeStructuredResultEnvelope(data)
		require.Error(t, err)
	}
	for _, mutate := range []func(*StructuredResultEnvelope){
		func(e *StructuredResultEnvelope) { e.Lease.Version = 1 },
		func(e *StructuredResultEnvelope) { e.Lease.ExecutedTugQL = "" },
		func(e *StructuredResultEnvelope) { e.Lease.ExecutedTugQL = strings.Repeat("x", 131073) },
		func(e *StructuredResultEnvelope) { e.Payload = []byte(`{}`) },
	} {
		copy := envelope
		mutate(&copy)
		data, err := json.Marshal(copy)
		require.NoError(t, err)
		_, err = DecodeStructuredResultEnvelope(data)
		require.Error(t, err)
	}
	for _, invalid := range []struct {
		authority *Authority
		ctx       context.Context
		started   time.Time
		op        StructuredRead
		request   ResultRequest
	}{
		{nil, context.Background(), started, op, request},
		{a, nil, started, op, request},
		{a, context.Background(), time.Time{}, op, request},
		{a, context.Background(), time.Now().Add(time.Hour), op, request},
		{a, context.Background(), started, StructuredRead{}, request},
		{a, context.Background(), started, op, resultRequest()},
	} {
		copy := envelope
		copy.Payload = bytes.Clone(envelope.Payload)
		_, err := invalid.authority.ImportStructuredResult(invalid.ctx, invalid.request, invalid.started, invalid.op, copy)
		require.Error(t, err)
	}
	badOp := op
	badOp.TugQL = "bad: query"
	badRequest := structuredRequest(t, badOp)
	bad := envelope
	bad.Lease.Operation = badOp
	bad.Lease.Nonce = badRequest.Nonce
	bad.Lease.RequestSHA256 = badRequest.RequestSHA256
	bad.Payload = bytes.Clone(envelope.Payload)
	_, err = a.ImportStructuredResult(context.Background(), badRequest, started, badOp, bad)
	require.Error(t, err)
	copy := envelope
	copy.Payload = bytes.Clone(envelope.Payload)
	resolution.Grants[0].State = Revoked
	_, err = testAuthority(t, fixedResolver(resolution)).ImportStructuredResult(context.Background(), request, started, op, copy)
	require.Error(t, err)
}

func TestStructuredResultImportRetainsEffectiveResourceAuthority(t *testing.T) {
	envelope, started, resolution := structuredEnvelope(t)
	op := structuredOperation()
	request := structuredRequest(t, op)
	// Authenticated server constraints may add a subquery/source. Import must
	// not drop that actual operation's resource when checking retained release.
	envelope.Lease.ExecutedTugQL = "from: {schema: sales, name: Hidden}\ncolumns: [{field: id}]"
	envelope.Lease.ExecutionSHA256 = hashBytes([]byte(envelope.Lease.ExecutedTugQL))
	_, err := testAuthority(t, fixedResolver(resolution)).ImportStructuredResult(context.Background(), request, started, op, envelope)
	require.Error(t, err)
}

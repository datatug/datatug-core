package dataaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// StructuredResultLease is version3 metadata for the complete immutable
// structured operation. It is not a capability; v1 and table-v2 refuse it.
type StructuredResultLease struct {
	ResultLease
	Profile   string         `json:"profile"`
	Operation StructuredRead `json:"operation"`
	// ExecutedTugQL is the server-authenticated effective operation after
	// subordinate constraints. It is never imported as executable authority.
	// Its resources join authored resources for fresh retained-value checks.
	ExecutedTugQL   string `json:"executedTugql"`
	ExecutionSHA256 string `json:"executionSHA256"`
}

type StructuredResultEnvelope struct {
	Payload json.RawMessage       `json:"payload"`
	Lease   StructuredResultLease `json:"lease"`
}

func DecodeStructuredResultEnvelope(data []byte) (StructuredResultEnvelope, error) {
	var envelope StructuredResultEnvelope
	if len(data) > 2359296 {
		return envelope, fmt.Errorf("dataaccess: structured envelope exceeds byte limit")
	}
	err := tableDecode(data, &envelope)
	if err == nil && (envelope.Lease.Version != 3 || envelope.Lease.Profile != StructuredReadProfile || !envelope.Lease.Operation.valid() || envelope.Payload == nil || len(envelope.Lease.ExecutedTugQL) == 0 || len(envelope.Lease.ExecutedTugQL) > 131072 || envelope.Lease.ExecutionSHA256 != hashBytes([]byte(envelope.Lease.ExecutedTugQL))) {
		err = fmt.Errorf("dataaccess: structured envelope required")
	}
	if err == nil {
		err = validateTablePayload(envelope.Payload, TableRead{MaxRows: envelope.Lease.Operation.MaxRows, MaxBytes: envelope.Lease.Operation.MaxBytes})
	}
	if err != nil {
		clear(envelope.Payload)
		return StructuredResultEnvelope{}, err
	}
	return envelope, nil
}

func (a *Authority) newStructuredResult(ctx context.Context, request ResultRequest, operation StructuredRead, bound *lease, payload []byte) *Result {
	metadata := ResultLease{Version: 3, Subject: a.request.Subject, Connection: a.request.Connection, GrantSetDigest: bound.digest, Nonce: request.Nonce, RequestSHA256: request.RequestSHA256, PayloadSHA256: hashBytes(payload), ServerTime: bound.serverTime, ExpiresAt: bound.serverTime.Add(bound.deadline.Sub(bound.started))}
	result := &Result{authority: a, authorization: bound, metadata: metadata, structured: &operation, payload: payload, lifetime: ctx}
	result.watch()
	return result
}

// ImportStructuredResult requires authenticated transport and independently
// expected original intent, subject, source, nonce and original credential
// deadline. Started is recorded BEFORE the network request. Payload ownership
// transfers to Core and is cleared before fresh authority I/O on every path.
func (a *Authority) ImportStructuredResult(ctx context.Context, request ResultRequest, started time.Time, expected StructuredRead, envelope StructuredResultEnvelope) (*Result, error) {
	defer clear(envelope.Payload)
	if a == nil || a.resolve == nil || ctx == nil || started.IsZero() {
		return nil, fmt.Errorf("dataaccess: structured authority and original request required")
	}
	expectedRequest, err := expected.RequestFor(request.Nonce)
	if err != nil || expectedRequest != request {
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	metadata := envelope.Lease
	ttl := metadata.ExpiresAt.Sub(metadata.ServerTime)
	if metadata.Version != 3 || metadata.Profile != StructuredReadProfile || metadata.Operation != expected || metadata.Subject != a.request.Subject || !sameConnection(metadata.Connection, a.request.Connection) || metadata.Nonce != request.Nonce || metadata.RequestSHA256 != request.RequestSHA256 || !validHash(metadata.GrantSetDigest) || !validHash(metadata.PayloadSHA256) || metadata.ServerTime.IsZero() || ttl <= 0 || ttl > time.Minute || started.After(a.now()) {
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	_, readRequest, err := expected.query()
	if err != nil {
		return nil, err
	}
	if metadata.ExecutionSHA256 != hashBytes([]byte(metadata.ExecutedTugQL)) {
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	effective := StructuredRead{TugQL: metadata.ExecutedTugQL, MaxRows: expected.MaxRows, MaxBytes: expected.MaxBytes}
	_, actual, err := effective.query()
	if err != nil {
		return nil, err
	}
	readRequest.Resources = unionStructuredResources(readRequest.Resources, actual.Resources)
	payload, err := compactPayload(envelope.Payload)
	clear(envelope.Payload)
	if err != nil {
		return nil, err
	}
	if hashBytes(payload) != metadata.PayloadSHA256 {
		clear(payload)
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	bound := &lease{deadline: started.Add(ttl), started: started, serverTime: metadata.ServerTime, digest: metadata.GrantSetDigest, request: readRequest}
	clampContext(ctx, bound)
	result := a.newStructuredResult(ctx, request, expected, bound, payload)
	result.metadata = metadata.ResultLease
	result.executedTugQL = metadata.ExecutedTugQL
	result.mu.Lock()
	err = validateTablePayload(result.payload, TableRead{MaxRows: expected.MaxRows, MaxBytes: expected.MaxBytes})
	result.mu.Unlock()
	if err == nil {
		err = result.guard(ctx)
	}
	if err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

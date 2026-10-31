package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"time"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

// TableResultLease is an additive version2 binding for an exact Core operation.
// It is metadata, not a capability. v1 result decoders/importers refuse it.
type TableResultLease struct {
	ResultLease
	Profile   string    `json:"profile"`
	Operation TableRead `json:"operation"`
}

type TableResultEnvelope struct {
	Payload json.RawMessage  `json:"payload"`
	Lease   TableResultLease `json:"lease"`
}

// tableDecode reuses Core's duplicate/case-folded-key checker, preserving exact
// integer tokens. Do not change v1's decoder or decode record cells to float64.
func tableDecode(data []byte, target any) error {
	if err := jsonstrict.ValidateUnicode(data); err != nil {
		return err
	}
	if err := jsonstrict.CheckNoDuplicateKeysFor(data, reflect.TypeOf(target)); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("dataaccess: trailing table JSON")
	}
	return nil
}

func DecodeTableResultEnvelope(data []byte) (TableResultEnvelope, error) {
	var envelope TableResultEnvelope
	if len(data) > 294912 {
		return envelope, fmt.Errorf("dataaccess: table envelope exceeds byte limit")
	}
	err := tableDecode(data, &envelope)
	if err == nil && (envelope.Lease.Version != 2 || envelope.Lease.Profile != TableReadProfile || !envelope.Lease.Operation.valid() || envelope.Payload == nil) {
		err = fmt.Errorf("dataaccess: table envelope is required")
	}
	if err == nil {
		err = validateTablePayload(envelope.Payload, envelope.Lease.Operation)
	}
	if err != nil {
		clear(envelope.Payload)
		return TableResultEnvelope{}, err
	}
	return envelope, nil
}

func (a *Authority) newTableResult(ctx context.Context, request ResultRequest, operation TableRead, bound *lease, payload []byte) *Result {
	metadata := ResultLease{Version: 2, Subject: a.request.Subject, Connection: a.request.Connection, GrantSetDigest: bound.digest, Nonce: request.Nonce, RequestSHA256: request.RequestSHA256, PayloadSHA256: hashBytes(payload), ServerTime: bound.serverTime, ExpiresAt: bound.serverTime.Add(bound.deadline.Sub(bound.started))}
	result := &Result{authority: a, authorization: bound, metadata: metadata, table: &operation, payload: payload, lifetime: ctx}
	result.watch()
	return result
}

// ImportTableResult requires the same authenticated transport and original
// credential/context/deadline discipline as ImportResult, plus an independently
// expected closed operation. The host pins actual server source identity first;
// the client never fabricates native span facts. Ownership of Payload transfers
// to Core, and caller bytes are cleared before any fresh authority network I/O.
func (a *Authority) ImportTableResult(ctx context.Context, request ResultRequest, started time.Time, expected TableRead, envelope TableResultEnvelope) (*Result, error) {
	defer clear(envelope.Payload)
	if a == nil || a.resolve == nil || ctx == nil || !request.valid() || !expected.valid() || started.IsZero() {
		return nil, fmt.Errorf("dataaccess: table authority, operation and original request required")
	}
	metadata := envelope.Lease
	ttl := metadata.ExpiresAt.Sub(metadata.ServerTime)
	if metadata.Version != 2 || metadata.Profile != TableReadProfile || metadata.Operation != expected || metadata.Subject != a.request.Subject || metadata.Connection.Space != a.request.Connection.Space || metadata.Connection.ID != a.request.Connection.ID || !sameFingerprint(metadata.Connection.Fingerprint, a.request.Connection.Fingerprint) || metadata.Nonce != request.Nonce || metadata.RequestSHA256 != request.RequestSHA256 || !validHash(metadata.GrantSetDigest) || !validHash(metadata.PayloadSHA256) || metadata.ServerTime.IsZero() || ttl <= 0 || ttl > time.Minute || started.After(a.now()) {
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	payload, err := compactPayload(envelope.Payload)
	clear(envelope.Payload)
	if err != nil {
		return nil, err
	}
	if hashBytes(payload) != metadata.PayloadSHA256 {
		clear(payload)
		return nil, a.denial(DataRead, "RESULT_BINDING_MISMATCH")
	}
	_, readRequest, _ := expected.query() // valid expected DTO constructs this profile only
	bound := &lease{deadline: started.Add(ttl), started: started, serverTime: metadata.ServerTime, digest: metadata.GrantSetDigest, request: readRequest}
	clampContext(ctx, bound)
	result := a.newTableResult(ctx, request, expected, bound, payload)
	result.metadata = metadata.ResultLease
	// Validation is bounded CPU over closed built-in JSON types, never I/O.
	// Keep the purge owner from clearing the bytes while they are inspected.
	result.mu.Lock()
	err = validateTablePayload(result.payload, expected)
	result.mu.Unlock()
	if err != nil {
		result.Close()
		return nil, err
	}
	if err := result.guard(ctx); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

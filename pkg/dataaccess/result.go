package dataaccess

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"
)

// ResultRequest binds the exact request and a fresh unpredictable host/client
// nonce. The digest is SHA256 of the agreed canonical request bytes.
type ResultRequest struct {
	Nonce         string `json:"nonce"`
	RequestSHA256 string `json:"requestSHA256"`
}

// ResultLease is transport metadata, never a capability. ImportResult accepts
// it only from the host's authenticated transport, then resolves current facts.
// ExpiresAt-ServerTime is anchored before the original network request; receiving
// or refreshing metadata must never restart that interval.
type ResultLease struct {
	Version        int           `json:"version"`
	Subject        Subject       `json:"subject"`
	Connection     ConnectionRef `json:"connection"`
	GrantSetDigest string        `json:"grantSetDigest"`
	Nonce          string        `json:"nonce"`
	RequestSHA256  string        `json:"requestSHA256"`
	PayloadSHA256  string        `json:"payloadSHA256"`
	ServerTime     time.Time     `json:"serverTime"`
	ExpiresAt      time.Time     `json:"expiresAt"`
}

type ResultEnvelope struct {
	Payload json.RawMessage `json:"payload"`
	Lease   ResultLease     `json:"lease"`
}

// Result keeps DataTug-owned values private until a final release check. Values
// handed to an io.Writer are external copies and cannot be recalled. Writers
// must obey io.Writer's contract: neither modify nor retain the supplied buffer.
type Result struct {
	mu            sync.Mutex
	authority     *Authority
	authorization *lease
	metadata      ResultLease
	payload       []byte
	lifetime      context.Context
	timer         *time.Timer
	stopLifetime  func() bool
}

func hashBytes(value []byte) string { sum := sha256.Sum256(value); return hex.EncodeToString(sum[:]) }
func validHash(value string) bool {
	if len(value) != sha256.Size*2 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
func (request ResultRequest) valid() bool {
	return len(request.Nonce) >= 16 && len(request.Nonce) <= 256 && canonical(request.Nonce) && validHash(request.RequestSHA256)
}
func resolutionDigest(resolution Resolution) (string, error) {
	grants := slices.Clone(resolution.Grants)
	slices.SortFunc(grants, func(a, b Grant) int { return strings.Compare(a.ID, b.ID) })
	data, err := json.Marshal(struct {
		Protection ConnectionProtection
		Grants     []Grant
	}{resolution.Protection, grants})
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

// strictJSON rejects unknown fields (when decoded into a struct), duplicate
// keys at every depth, trailing documents and malformed JSON before use.
func strictJSON(data []byte, value any) error {
	tokens := json.NewDecoder(bytes.NewReader(data))
	tokens.UseNumber()
	if err := jsonValue(tokens); err != nil {
		return err
	}
	if _, err := tokens.Token(); err != io.EOF {
		return fmt.Errorf("dataaccess: trailing JSON")
	}
	if value == nil {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	return decoder.Decode(value)
}
func jsonValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	seen := make(map[string]bool)
	for decoder.More() {
		if delimiter == '{' {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok || seen[name] {
				return fmt.Errorf("dataaccess: duplicate JSON key")
			}
			seen[name] = true
		}
		if err := jsonValue(decoder); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}
func compactPayload(data []byte) ([]byte, error) {
	if err := strictJSON(data, nil); err != nil {
		return nil, err
	}
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("dataaccess: result payload must be an object")
	}
	var compact bytes.Buffer
	err := json.Compact(&compact, data)
	return compact.Bytes(), err
}

func DecodeResultEnvelope(data []byte) (ResultEnvelope, error) {
	var envelope ResultEnvelope
	if err := strictJSON(data, &envelope); err != nil {
		return ResultEnvelope{}, err
	}
	if envelope.Lease.Version != 1 || envelope.Payload == nil {
		return ResultEnvelope{}, fmt.Errorf("dataaccess: result envelope is required")
	}
	return envelope, nil
}

func clampContext(ctx context.Context, authorization *lease) {
	if deadline, ok := ctx.Deadline(); ok && deadline.Before(authorization.deadline) {
		authorization.deadline = deadline
	}
}

// CaptureResult is a trusted host composition point. read must use the secured
// database for this Authority and transfer ownership of the complete serialized
// JSON payload buffer (Core clears it, including on read/validation failure).
// The first resolve precedes I/O, and neither I/O nor later release renews it.
// This transport checkpoint requires connection-wide read authority; object-only
// results require a separately reviewed resource-bound transport contract.
func (a *Authority) CaptureResult(ctx context.Context, request ResultRequest, read func(context.Context) ([]byte, error)) (*Result, error) {
	if a == nil || a.resolve == nil || !request.valid() || read == nil {
		return nil, fmt.Errorf("dataaccess: result authority, request and reader required")
	}
	prepared, bound, cancel, err := a.prepare(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer cancel()
	clampContext(ctx, bound)
	data, err := read(prepared)
	defer clear(data)
	if err != nil {
		return nil, err
	}
	payload, err := compactPayload(data)
	clear(data)
	if err != nil {
		return nil, err
	}
	metadata := ResultLease{Version: 1, Subject: a.request.Subject, Connection: a.request.Connection, GrantSetDigest: bound.digest, Nonce: request.Nonce, RequestSHA256: request.RequestSHA256, PayloadSHA256: hashBytes(payload), ServerTime: bound.serverTime}
	result := &Result{authority: a, authorization: bound, metadata: metadata, payload: payload, lifetime: ctx}
	result.metadata.ExpiresAt = bound.serverTime.Add(bound.deadline.Sub(bound.started))
	result.watch()
	if err := result.guard(ctx); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

// ImportResult requires an authenticated trusted server envelope and independently
// expected subject, connection/scope, request digest and fresh nonce. The host
// pins the actual source identity from that trusted transport before constructing
// Authority. requestStarted MUST be captured before the HTTP request. The caller
// must bound ctx by the verified ORIGINAL credential deadline, not its refresh.
// Object grants never substitute for current connection-wide result authority.
// Ownership of envelope.Payload transfers to Core; it is cleared on every path
// and immediately after compaction, before any authority network I/O.
func (a *Authority) ImportResult(ctx context.Context, request ResultRequest, requestStarted time.Time, envelope ResultEnvelope) (*Result, error) {
	defer clear(envelope.Payload)
	if a == nil || a.resolve == nil || !request.valid() || requestStarted.IsZero() {
		return nil, fmt.Errorf("dataaccess: result authority and original request required")
	}
	metadata := envelope.Lease
	ttl := metadata.ExpiresAt.Sub(metadata.ServerTime)
	if metadata.Version != 1 || metadata.Subject != a.request.Subject || metadata.Connection.Space != a.request.Connection.Space || metadata.Connection.ID != a.request.Connection.ID || !sameFingerprint(metadata.Connection.Fingerprint, a.request.Connection.Fingerprint) || metadata.Nonce != request.Nonce || metadata.RequestSHA256 != request.RequestSHA256 || !validHash(metadata.GrantSetDigest) || !validHash(metadata.PayloadSHA256) || metadata.ServerTime.IsZero() || ttl <= 0 || ttl > time.Minute || requestStarted.After(a.now()) {
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
	bound := &lease{deadline: requestStarted.Add(ttl), started: requestStarted, serverTime: metadata.ServerTime, digest: metadata.GrantSetDigest, policy: readPolicy}
	clampContext(ctx, bound)
	result := &Result{authority: a, authorization: bound, metadata: metadata, payload: payload, lifetime: ctx}
	result.watch()
	if err := result.guard(ctx); err != nil {
		result.Close()
		return nil, err
	}
	return result, nil
}

func (r *Result) watch() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timer = time.AfterFunc(time.Until(r.authorization.deadline), func() { r.mu.Lock(); defer r.mu.Unlock(); r.purgeLocked() })
	r.stopLifetime = context.AfterFunc(r.lifetime, func() { r.mu.Lock(); defer r.mu.Unlock(); r.purgeLocked() })
}
func (r *Result) purgeLocked() {
	clear(r.payload)
	r.payload = nil
	if r.timer != nil {
		r.timer.Stop()
	}
	if r.stopLifetime != nil {
		r.stopLifetime()
	}
}

// Close discards retained values even if no subsequent release is requested.
func (r *Result) Close() {
	if r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.purgeLocked()
	}
}
func (r *Result) guard(ctx context.Context) error {
	r.mu.Lock()
	if r.payload == nil {
		r.mu.Unlock()
		return fmt.Errorf("dataaccess: result is closed")
	}
	bound := *r.authorization
	r.mu.Unlock()
	if err := r.lifetime.Err(); err != nil {
		return err
	}
	// Bound authority I/O before entering the resolver. An uncooperative resolver
	// still cannot block the independent timer/lifetime purge or Close.
	resolving, cancel := context.WithDeadline(ctx, bound.deadline)
	stop := context.AfterFunc(r.lifetime, cancel)
	defer func() { stop(); cancel() }()
	_, current, stopPrepared, err := r.authority.prepare(resolving, &bound)
	if err != nil {
		return err
	}
	defer stopPrepared()
	if current.digest != bound.digest {
		return r.authority.denial(DataRead, "RESULT_AUTHORITY_CHANGED")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.payload == nil {
		return fmt.Errorf("dataaccess: result is closed")
	}
	if current.deadline.Before(r.authorization.deadline) {
		r.authorization.deadline = current.deadline
	}
	if r.timer != nil {
		r.timer.Reset(time.Until(r.authorization.deadline))
	}
	return r.authority.check(ctx, r.authorization)
}
func (r *Result) write(ctx context.Context, writer io.Writer, envelope bool) error {
	if r == nil || writer == nil {
		return fmt.Errorf("dataaccess: result and writer required")
	}
	if err := r.guard(ctx); err != nil {
		r.Close()
		return err
	}
	return r.writeAuthorized(ctx, writer, envelope)
}

// writeAuthorized reacquires ownership after the network phase; Close may have
// purged the values while authority was being resolved.
func (r *Result) writeAuthorized(ctx context.Context, writer io.Writer, envelope bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.payload == nil {
		return fmt.Errorf("dataaccess: result is closed")
	}
	data := r.payload
	if envelope {
		// Preserve the original ceiling if a current resolve has shortened the lease.
		metadata := r.metadata
		metadata.ExpiresAt = metadata.ServerTime.Add(r.authorization.deadline.Sub(r.authorization.started))
		var err error
		data, err = json.Marshal(ResultEnvelope{Payload: r.payload, Lease: metadata})
		if err != nil {
			r.purgeLocked()
			return err
		}
		defer clear(data)
	}
	// Serialization can run arbitrary time; no writer receives bytes until this
	// last local cancellation/deadline check. Network revocation was checked above.
	if err := r.lifetime.Err(); err != nil {
		r.purgeLocked()
		return err
	}
	if err := r.authority.check(ctx, r.authorization); err != nil {
		r.purgeLocked()
		return err
	}
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}
func (r *Result) WriteEnvelope(ctx context.Context, writer io.Writer) error {
	return r.write(ctx, writer, true)
}
func (r *Result) WritePayload(ctx context.Context, writer io.Writer) error {
	return r.write(ctx, writer, false)
}

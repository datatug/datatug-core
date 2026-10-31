package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/stretchr/testify/require"
)

type tableSourceFunc func(context.Context, TableIdentityRequest) (TableReadSpan, error)

func (f tableSourceFunc) OpenTableRead(ctx context.Context, r TableIdentityRequest) (TableReadSpan, error) {
	return f(ctx, r)
}

type previewSpan struct {
	session       dal.ReadSession
	facts         TableIdentityFacts
	verify        func(context.Context) error
	stop          func()
	closeErr      error
	closed        atomic.Bool
	verifications atomic.Int32
}

func (s *previewSpan) Session() dal.ReadSession { return s.session }
func (s *previewSpan) Verify(ctx context.Context, _ TableIdentityRequest) (TableIdentityFacts, error) {
	s.verifications.Add(1)
	if s.closed.Load() {
		return TableIdentityFacts{}, errors.New("closed span")
	}
	if s.verify != nil {
		if err := s.verify(ctx); err != nil {
			return TableIdentityFacts{}, err
		}
	}
	return s.facts, ctx.Err()
}
func (s *previewSpan) Close() error {
	if s.closed.CompareAndSwap(false, true) && s.stop != nil {
		s.stop()
	}
	return s.closeErr
}

type previewReader struct {
	rs        recordset.Recordset
	index     int
	next      func() (recordset.Row, recordset.Recordset, error)
	closeHook func() error
	closes    atomic.Int32
}

func (r *previewReader) Recordset() recordset.Recordset { return r.rs }
func (r *previewReader) Cursor() (string, error)        { return "", nil }
func (r *previewReader) Close() error {
	r.closes.Add(1)
	if r.closeHook != nil {
		return r.closeHook()
	}
	return nil
}
func (r *previewReader) Next() (recordset.Row, recordset.Recordset, error) {
	if r.next != nil {
		return r.next()
	}
	if r.index == r.rs.RowsCount() {
		return nil, nil, dal.ErrNoMoreRecords
	}
	row := r.rs.GetRow(r.index)
	r.index++
	return row, r.rs, nil
}

type previewAdapter struct {
	dal.ReadSession
	reader dal.RecordsetReader
	err    error
	calls  atomic.Int32
	query  dal.StructuredQuery
}

func (s *previewAdapter) ExecuteQueryToRecordsetReader(_ context.Context, q dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	s.calls.Add(1)
	s.query = q.(dal.StructuredQuery)
	return s.reader, s.err
}

// A distinct recording implementation uses an execution function rather than a
// retained reader field; both must consume the same Core-built operation.
type previewFunctionAdapter struct {
	dal.ReadSession
	execute func(dal.Query) (dal.RecordsetReader, error)
}

func (s previewFunctionAdapter) ExecuteQueryToRecordsetReader(_ context.Context, q dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return s.execute(q)
}

func previewOperation() TableRead {
	return TableRead{Object: ObjectRef{Namespace: "sales", Name: "Customer"}, MaxRows: 2, MaxBytes: 4096}
}
func previewRecordset(t *testing.T, names []string, rows ...[]any) recordset.Recordset {
	t.Helper()
	cols := make([]recordset.Column[any], len(names))
	for i, n := range names {
		cols[i] = recordset.NewTypedColumn[any](n, nil)
	}
	rs := recordset.NewColumnarRecordset("native-table", cols...)
	for _, values := range rows {
		row := rs.NewRow()
		for i, v := range values {
			require.NoError(t, row.SetValueByIndex(i, v, rs))
		}
	}
	return rs
}
func previewSource(span *previewSpan) tableSourceFunc {
	return func(context.Context, TableIdentityRequest) (TableReadSpan, error) { return span, nil }
}
func newPreviewSpan(session dal.ReadSession) *previewSpan {
	return &previewSpan{session: session, facts: TableIdentityFacts{Connection: fixtureConnection(), Table: access.TableName{Schema: "sales", Name: "Customer"}}}
}
func previewResult(t *testing.T, a *Authority, ctx context.Context, op TableRead, span *previewSpan) *Result {
	t.Helper()
	r, err := a.ReadTableResult(ctx, resultRequest(), op, previewSource(span))
	require.NoError(t, err)
	t.Cleanup(r.Close)
	return r
}

func TestTablePreviewTwoAdaptersDataAndVersionedTransport(t *testing.T) {
	for _, function := range []bool{false, true} {
		t.Run(map[bool]string{false: "reader-adapter", true: "function-adapter"}[function], func(t *testing.T) {
			rs := previewRecordset(t, []string{"id", "text", "null", "blob", "real", "bool"}, []any{int64(math.MaxInt64), "Ada", nil, []byte{}, 1.25, true}, []any{int64(2), "Grace", nil, []byte{0, 255}, 2.5, false})
			reader := &previewReader{rs: rs}
			adapter := &previewAdapter{reader: reader}
			var session dal.ReadSession = adapter
			if function {
				session = previewFunctionAdapter{execute: func(q dal.Query) (dal.RecordsetReader, error) {
					return adapter.ExecuteQueryToRecordsetReader(context.Background(), q)
				}}
			}
			span := newPreviewSpan(session)
			a := testAuthority(t, fixedResolver(qualifiedResolution()))
			started := time.Now()
			r := previewResult(t, a, context.Background(), previewOperation(), span)
			require.EqualValues(t, 1, reader.closes.Load())
			require.False(t, span.closed.Load())
			require.EqualValues(t, 1, adapter.calls.Load())
			require.Equal(t, 3, adapter.query.Limit())
			require.Empty(t, adapter.query.Columns())
			require.Nil(t, adapter.query.Where())
			require.Empty(t, adapter.query.From().Joins())
			require.Equal(t, previewOperation().Object.Namespace, adapter.query.From().Base().(dal.CollectionRef).Schema())
			var wire bytes.Buffer
			require.NoError(t, r.WriteEnvelope(context.Background(), &wire))
			require.Error(t, validateTablePayload([]byte(`{}`), previewOperation()))
			env, err := DecodeTableResultEnvelope(wire.Bytes())
			require.NoError(t, err)
			require.Equal(t, 2, env.Lease.Version)
			require.Equal(t, TableReadProfile, env.Lease.Profile)
			require.Equal(t, previewOperation(), env.Lease.Operation)
			require.Contains(t, string(env.Payload), `9223372036854775807`)
			require.Contains(t, string(env.Payload), `{"base64":"AP8="}`)
			require.Contains(t, string(env.Payload), `"complete":true`)
			_, err = DecodeResultEnvelope(wire.Bytes())
			require.Error(t, err)
			imported, err := a.ImportTableResult(context.Background(), resultRequest(), started, previewOperation(), env)
			require.NoError(t, err)
			defer imported.Close()
			require.NotContains(t, string(env.Payload), "Ada")
			var payload bytes.Buffer
			require.NoError(t, imported.WritePayload(context.Background(), &payload))
			require.Contains(t, payload.String(), "Ada")
			require.NoError(t, validateTablePayload(payload.Bytes(), previewOperation()))
			r.Close()
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
		})
	}
}

func TestTablePreviewEmptyExactAndExtraRowAreLabelled(t *testing.T) {
	for _, count := range []int{0, 2, 3} {
		t.Run(string(rune('0'+count)), func(t *testing.T) {
			rows := make([][]any, count)
			for i := range rows {
				rows[i] = []any{int64(i)}
			}
			reader := &previewReader{rs: previewRecordset(t, []string{"id"}, rows...)}
			r := previewResult(t, testAuthority(t, fixedResolver(qualifiedResolution())), context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: reader}))
			var out bytes.Buffer
			require.NoError(t, r.WritePayload(context.Background(), &out))
			var value struct {
				Rows    [][]any
				Preview struct {
					ReturnedRows uint32
					HasMore      bool
					Complete     bool
				}
			}
			require.NoError(t, json.Unmarshal(out.Bytes(), &value))
			require.Len(t, value.Rows, min(count, 2))
			require.Equal(t, count > 2, value.Preview.HasMore)
			require.Equal(t, count <= 2, value.Preview.Complete)
			require.NoError(t, validateTablePayload(out.Bytes(), previewOperation()))
		})
	}
}

func TestTablePreviewAdmissionAndContextPoliciesFailBeforeDispatch(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	var calls int
	source := tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) {
		calls++
		return nil, errors.New("secret native credential")
	})
	for _, op := range []TableRead{{}, {Object: ObjectRef{Namespace: "sales", Name: "Other"}, MaxRows: 1, MaxBytes: 1000}, {Object: previewOperation().Object, MaxRows: 101, MaxBytes: 1000}, {Object: previewOperation().Object, MaxRows: 1, MaxBytes: 262145}} {
		_, err := a.ReadTableResult(context.Background(), resultRequest(), op, source)
		require.Error(t, err)
	}
	require.Zero(t, calls)
	_, err := a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), source)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
	for _, source := range []TableReadSource{nil, tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) { return nil, nil })} {
		_, err = a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), source)
		require.Error(t, err)
	}
	var missing *Authority
	_, err = missing.ReadTableResult(context.Background(), resultRequest(), previewOperation(), source)
	require.Error(t, err)
	_, err = a.ReadTableResult(context.Background(), ResultRequest{}, previewOperation(), source)
	require.Error(t, err)
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
	span := newPreviewSpan(adapter)
	ctx := access.WithPolicy(context.Background(), access.MustPolicy("deny-all", access.Root(access.Deny(access.Read))))
	_, err = a.ReadTableResult(ctx, resultRequest(), previewOperation(), previewSource(span))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Zero(t, adapter.calls.Load())
}

func TestTablePreviewNativeFailuresAndOwnedClose(t *testing.T) {
	for _, mode := range []string{"span-error", "span-nil-session", "reader-error", "reader-nil", "reader-close", "fact-error", "fact-name", "fact-source", "next-error", "row-nil", "columns-changed", "value-width", "value-error", "unknown-cell"} {
		t.Run(mode, func(t *testing.T) {
			a := testAuthority(t, fixedResolver(qualifiedResolution()))
			rs := previewRecordset(t, []string{"id"}, []any{int64(1)})
			reader := &previewReader{rs: rs}
			adapter := &previewAdapter{reader: reader}
			span := newPreviewSpan(adapter)
			source := previewSource(span)
			switch mode {
			case "span-error":
				source = func(context.Context, TableIdentityRequest) (TableReadSpan, error) { return span, errors.New("secret") }
			case "span-nil-session":
				span.session = nil
			case "reader-error":
				adapter.err = errors.New("secret")
			case "reader-nil":
				adapter.reader = nil
			case "reader-close":
				reader.closeHook = func() error { return errors.New("secret") }
			case "fact-error":
				span.verify = func(context.Context) error { return errors.New("secret") }
			case "fact-name":
				span.facts.Table.Name = "Other"
			case "fact-source":
				span.facts.Connection.Fingerprint.EngineIdentity = "other"
			case "next-error":
				reader.next = func() (recordset.Row, recordset.Recordset, error) { return nil, nil, errors.New("secret") }
			case "row-nil":
				reader.next = func() (recordset.Row, recordset.Recordset, error) { return nil, rs, nil }
			case "columns-changed":
				other := previewRecordset(t, []string{"other"}, []any{int64(1)})
				reader.next = func() (recordset.Row, recordset.Recordset, error) { return other.GetRow(0), other, nil }
			case "value-width":
				reader.next = func() (recordset.Row, recordset.Recordset, error) { return previewBadRow{values: []any{}}, rs, nil }
			case "value-error":
				reader.next = func() (recordset.Row, recordset.Recordset, error) {
					return previewBadRow{err: errors.New("secret")}, rs, nil
				}
			case "unknown-cell":
				reader.next = func() (recordset.Row, recordset.Recordset, error) {
					return previewBadRow{values: []any{make(chan int)}}, rs, nil
				}
			}
			r, err := a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), source)
			require.Error(t, err)
			require.Nil(t, r)
			require.NotContains(t, err.Error(), "secret")
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
			if mode == "reader-error" || mode == "reader-close" || mode == "next-error" {
				require.Eventually(t, func() bool { return reader.closes.Load() == 1 }, time.Second, time.Millisecond)
			}
		})
	}
}

type previewBadRow struct {
	recordset.Row
	values []any
	err    error
}

func (r previewBadRow) Data(recordset.Recordset) ([]any, error) { return r.values, r.err }

func TestTablePreviewRealDeadlinePurgesWhileNextStalls(t *testing.T) {
	resolution := qualifiedResolution()
	resolution.ServerTime = time.Now()
	resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(100 * time.Millisecond)
	a := testAuthority(t, fixedResolver(resolution))
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	reader := &previewReader{rs: previewRecordset(t, []string{"id"})}
	reader.next = func() (recordset.Row, recordset.Recordset, error) {
		close(entered)
		<-release
		return nil, nil, dal.ErrNoMoreRecords
	}
	reader.closeHook = func() error { once.Do(func() { close(release) }); return nil }
	span := newPreviewSpan(&previewAdapter{reader: reader})
	done := make(chan error, 1)
	go func() {
		r, err := a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), previewSource(span))
		if r != nil {
			r.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("native Next was not entered: %v", err)
	case <-time.After(time.Second):
		t.Fatal("native Next did not start")
	}
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("expiry did not close blocked Next")
	}
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
}

func TestTablePreviewRevocationAndReplacementCannotRelease(t *testing.T) {
	for _, mode := range []string{"revoke", "replacement", "added"} {
		t.Run(mode, func(t *testing.T) {
			resolution := qualifiedResolution()
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
			span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
			r := previewResult(t, a, context.Background(), previewOperation(), span)
			switch mode {
			case "revoke":
				resolution.Grants[0].State = Revoked
			case "replacement":
				resolution.Grants[0].ID = "B"
				resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(50 * time.Second)
			case "added":
				later := resolution.Grants[0]
				later.ID = "B"
				later.ExpiresAt = resolution.ServerTime.Add(50 * time.Second)
				resolution.Grants = append(resolution.Grants, later)
			}
			if mode != "revoke" {
				_, request, _ := previewOperation().query()
				_, err := a.authorizeRequest(context.Background(), request)
				require.NoError(t, err, "a new operation would be allowed, but the retained A result must not be rescued")
			}
			var out bytes.Buffer
			require.Error(t, r.WritePayload(context.Background(), &out))
			require.Zero(t, out.Len())
			assertResultPurged(t, r)
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
		})
	}
}

func TestTablePreviewRetainedCloseDoesNotWaitForFreshResolver(t *testing.T) {
	resolution := qualifiedResolution()
	var stalled atomic.Bool
	entered := make(chan struct{})
	release := make(chan struct{})
	a := testAuthority(t, func(ctx context.Context, _ ResolveRequest) (Resolution, error) {
		if stalled.Load() {
			close(entered)
			<-release
		}
		return resolution, nil
	})
	span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
	r := previewResult(t, a, context.Background(), previewOperation(), span)
	stalled.Store(true)
	done := make(chan error, 1)
	go func() { done <- r.WritePayload(context.Background(), io.Discard) }()
	<-entered
	r.Close()
	assertResultPurged(t, r)
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
	close(release)
	require.Error(t, <-done)
}

func TestTablePreviewAgentUsesBaselineAndNeverHumanGrants(t *testing.T) {
	for _, mode := range []string{"all", "none", "human-grant", "agent-none"} {
		t.Run(mode, func(t *testing.T) {
			resolution := fixtureResolution()
			subject := fixtureSubject()
			subject.Kind = Agent
			resolution.Subject = subject
			if mode == "none" || mode == "human-grant" {
				resolution.Protection.ReadPosture = ReadNone
			}
			if mode == "human-grant" {
				resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
			}
			if mode == "agent-none" {
				resolution.Protection.AgentRead = AgentNone
			}
			a, err := NewAuthority(subject, fixtureConnection(), fixedResolver(resolution))
			require.NoError(t, err)
			adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}
			span := newPreviewSpan(adapter)
			var opened int
			source := tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) { opened++; return span, nil })
			r, err := a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), source)
			if mode == "all" {
				require.NoError(t, err)
				defer r.Close()
				held := r.payload
				require.Equal(t, int(previewOperation().MaxBytes), cap(held))
				var out bytes.Buffer
				require.NoError(t, r.WritePayload(context.Background(), &out))
				require.Contains(t, out.String(), `[[1]]`)
				r.Close()
				require.Equal(t, make([]byte, len(held)), held)
			} else {
				require.Error(t, err)
				require.Nil(t, r)
				require.Zero(t, opened)
				require.Zero(t, adapter.calls.Load())
			}
		})
	}
}

func TestTablePreviewRestrictiveFieldResidualCannotRewriteClosedOperation(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id", "name"}, []any{int64(1), "Ada"})}}
	span := newPreviewSpan(adapter)
	restriction := access.MustPolicy("private-fields", access.TableScope(access.TableName{Schema: "sales", Name: "Customer"}, access.Allow(access.Read).Fields("id")))
	ctx := access.WithPolicy(context.Background(), restriction)
	r, err := a.ReadTableResult(ctx, resultRequest(), previewOperation(), previewSource(span))
	require.Error(t, err)
	require.Nil(t, r)
	require.Zero(t, adapter.calls.Load(), "no silently projected query may reach the native reader")
}

package dataaccess

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/tugql"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

type structuredSourceFunc func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error)

func (f structuredSourceFunc) OpenStructuredRead(ctx context.Context, request StructuredIdentityRequest) (StructuredReadSpan, error) {
	return f(ctx, request)
}

type structuredTestSpan struct {
	session dal.ReadSession
	facts   StructuredIdentityFacts
	verify  func(context.Context, StructuredIdentityRequest) error
	closed  atomic.Bool
	stop    func()
}

func (s *structuredTestSpan) Session() dal.ReadSession { return s.session }
func (s *structuredTestSpan) Verify(ctx context.Context, request StructuredIdentityRequest) (StructuredIdentityFacts, error) {
	if s.verify != nil {
		if err := s.verify(ctx, request); err != nil {
			return StructuredIdentityFacts{}, err
		}
	}
	if s.closed.Load() {
		return StructuredIdentityFacts{}, errors.New("closed")
	}
	return s.facts, ctx.Err()
}
func (s *structuredTestSpan) Close() error {
	if s.closed.CompareAndSwap(false, true) && s.stop != nil {
		s.stop()
	}
	return nil
}

func structuredOperation() StructuredRead {
	return StructuredRead{TugQL: "from: {schema: sales, name: Customer}\ncolumns: [{field: id}]\nlimit: 2", MaxRows: 2, MaxBytes: 4096}
}
func structuredRequest(t *testing.T, op StructuredRead) ResultRequest {
	t.Helper()
	request, err := op.RequestFor("independent-fresh-nonce-123")
	require.NoError(t, err)
	return request
}
func structuredSpan(session dal.ReadSession) *structuredTestSpan {
	return &structuredTestSpan{session: session, facts: StructuredIdentityFacts{Connection: fixtureConnection(), Resources: []StructuredResourceIdentity{{Table: access.TableName{Schema: "sales", Name: "Customer"}}}}}
}
func structuredResult(t *testing.T, a *Authority, ctx context.Context, op StructuredRead, span *structuredTestSpan) *Result {
	t.Helper()
	r, err := a.ReadStructuredResult(ctx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { return span, nil }))
	require.NoError(t, err)
	t.Cleanup(r.Close)
	return r
}

func TestStructuredReadPreservesCompleteOperationsAndTypedBindings(t *testing.T) {
	queries := []string{
		"from: {schema: sales, name: Customer}\ncolumns: [{field: name, as: display}]\nwhere: {op: '>=', left: {field: id}, right: {param: min}}\norderBy: [{field: id, desc: true}]\nlimit: 2\noffset: 1",
		"from: {schema: sales, name: Customer}\ncolumns: [{field: country}, {aggregate: {function: sum, args: [{field: total}]}, as: revenue}]\ngroupBy: [{field: country}]\nhaving: {op: '>', left: {field: revenue}, right: {value: 4}}\norderBy: [{field: revenue, desc: true}]\nlimit: 2",
		"from:\n  schema: sales\n  name: Customer\n  alias: c\n  joins:\n    - type: inner\n      from: {schema: sales, name: Order, alias: o}\n      on: [{op: '==', left: {field: id, source: c}, right: {field: customer, source: o}}]\ncolumns: [{field: name, source: c, as: customer}, {field: total, source: o, as: total}]\norderBy: [{field: total, source: o, desc: true}]\nlimit: 2",
	}
	for i, text := range queries {
		for _, second := range []bool{false, true} {
			op := structuredOperation()
			op.TugQL = text
			if i == 0 {
				op.Parameters = `{"min":{"type":"integer","value":"7"}}`
			}
			adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"value"}, []any{int64(7)})}}
			var session dal.ReadSession = adapter
			if second {
				session = previewFunctionAdapter{execute: func(q dal.Query) (dal.RecordsetReader, error) {
					return adapter.ExecuteQueryToRecordsetReader(context.Background(), q)
				}}
			}
			span := structuredSpan(session)
			if i == 2 {
				span.facts.Resources = append(span.facts.Resources, StructuredResourceIdentity{Table: access.TableName{Schema: "sales", Name: "Order"}})
			}
			r := structuredResult(t, testAuthority(t, fixedResolver(qualifiedResolution())), context.Background(), op, span)
			expected, _, err := op.query()
			require.NoError(t, err)
			require.True(t, tugql.Equal(expected, adapter.query), "the exact fully frozen operation reaches both drivers")
			if i == 0 {
				require.Equal(t, dal.Constant{Value: 7}, adapter.query.Where().(dal.Comparison).Right)
			}
			var out bytes.Buffer
			require.NoError(t, r.WritePayload(context.Background(), &out))
			require.Contains(t, out.String(), `[[7]]`)
		}
	}
}

func TestStructuredReadAdmissionDeniesBeforeOpen(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	opened := 0
	source := structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
		opened++
		return nil, errors.New("secret")
	})
	for _, text := range []string{
		"from: {schema: sales, name: Hidden}",
		"from: {schema: sales, name: Customer, joins: [{type: inner, from: {schema: sales, name: Hidden}, on: [{op: '==', left: {field: id}, right: {field: id}}]}]}",
		"from: {database: other, schema: sales, name: Customer}",
		"from: {schema: sales, name: Customer}\ncolumns: [{aggregate: {function: unknown, args: [{star: true}]}, as: x}]",
		"from: {schema: sales, name: Customer}\nopaque: true",
		"from: {schema: sales, name: Customer}\ncolumns: [{param: missing, as: x}]",
		"from: {schema: sales, name: Customer}\nwhere: {op: '==', left: {field: id}, right: {param: missing}}",
		"from: {name: 'spaces/{SneatUserID}/items'}",
		"not: a-query",
	} {
		op := structuredOperation()
		op.TugQL = text
		_, err := a.ReadStructuredResult(context.Background(), structuredRequest(t, op), op, source)
		require.Error(t, err, text)
	}
	require.Zero(t, opened)
	op := structuredOperation()
	_, err := a.ReadStructuredResult(context.Background(), resultRequest(), op, source)
	require.Error(t, err)
	_, err = (*Authority)(nil).ReadStructuredResult(context.Background(), ResultRequest{}, op, source)
	require.Error(t, err)
	for _, missingContext := range []context.Context{nil} {
		_, err = a.ReadStructuredResult(missingContext, ResultRequest{}, op, source)
		require.Error(t, err)
	}
	_, err = a.ReadStructuredResult(context.Background(), ResultRequest{}, op, nil)
	require.Error(t, err)
	_, err = a.ReadStructuredResult(context.Background(), structuredRequest(t, op), op, source)
	require.Error(t, err)
	require.NotContains(t, err.Error(), "secret")
}

func TestStructuredReadNativeFailureClosesOwnedResources(t *testing.T) {
	for _, mode := range []string{"source-error", "source-nil", "session-nil", "reader-error", "reader-nil", "close-error", "fact-error", "fact-source", "fact-joined-missing", "next-error", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			op := structuredOperation()
			reader := &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}
			adapter := &previewAdapter{reader: reader}
			span := structuredSpan(adapter)
			source := structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { return span, nil })
			switch mode {
			case "source-error":
				source = func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
					return span, errors.New("secret")
				}
			case "source-nil":
				source = func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { return nil, nil }
			case "session-nil":
				span.session = nil
			case "reader-error":
				adapter.err = errors.New("secret")
			case "reader-nil":
				adapter.reader = nil
			case "close-error":
				reader.closeHook = func() error { return errors.New("secret") }
			case "fact-error":
				span.verify = func(context.Context, StructuredIdentityRequest) error { return errors.New("secret") }
			case "fact-source":
				span.facts.Connection.Fingerprint.EngineIdentity = "wrong"
			case "fact-joined-missing":
				span.facts.Resources = nil
			case "next-error":
				reader.next = func() (recordset.Row, recordset.Recordset, error) { return nil, nil, errors.New("secret") }
			case "bytes":
				op.MaxBytes = 1
			}
			r, err := testAuthority(t, fixedResolver(qualifiedResolution())).ReadStructuredResult(context.Background(), structuredRequest(t, op), op, source)
			require.Error(t, err)
			require.Nil(t, r)
			require.NotContains(t, err.Error(), "secret")
			if mode != "source-nil" {
				require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
			}
		})
	}
}

func TestStructuredReadAgentBaselineAndContextIdentity(t *testing.T) {
	for _, mode := range []string{"all", "none", "human-grant", "agent-grant", "agent-none"} {
		t.Run(mode, func(t *testing.T) {
			resolution := fixtureResolution()
			resolution.Subject.Kind = Agent
			if mode == "none" || mode == "human-grant" || mode == "agent-grant" {
				resolution.Protection.ReadPosture = ReadNone
			}
			if mode == "human-grant" || mode == "agent-grant" {
				resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
				if mode == "agent-grant" {
					resolution.Grants[0].Subject.Kind = Agent
				}
			}
			if mode == "agent-none" {
				resolution.Protection.AgentRead = AgentNone
			}
			a, err := NewAuthority(resolution.Subject, resolution.Connection, fixedResolver(resolution))
			require.NoError(t, err)
			ctx := access.WithPrincipal(context.Background(), access.Principal{ID: "sam", Roles: []string{"owner"}, Groups: []string{"staff"}})
			adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
			span := structuredSpan(adapter)
			span.verify = func(ctx context.Context, _ StructuredIdentityRequest) error {
				p, ok := access.PrincipalFrom(ctx)
				require.True(t, ok)
				require.Equal(t, access.PrincipalKindAgent, p.Subject.Kind)
				require.Empty(t, p.Roles)
				require.Empty(t, p.Groups)
				require.Nil(t, p.ID)
				return nil
			}
			opened := 0
			source := structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
				opened++
				return span, nil
			})
			op := structuredOperation()
			r, err := a.ReadStructuredResult(ctx, structuredRequest(t, op), op, source)
			if mode == "all" {
				require.NoError(t, err)
				r.Close()
			} else {
				require.Error(t, err)
				require.Zero(t, opened)
				require.Zero(t, adapter.calls.Load())
			}
		})
	}
}

func TestStructuredReadRetainedInvalidationAndOriginalCeiling(t *testing.T) {
	for _, mode := range []string{"revoke", "membership", "source", "replace", "cancel", "expiry"} {
		t.Run(mode, func(t *testing.T) {
			resolution := qualifiedResolution()
			resolution.ServerTime = time.Now()
			resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
			resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(150 * time.Millisecond)
			missing := false
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				if missing {
					return Resolution{}, errors.New("lost membership")
				}
				return resolution, nil
			})
			span := structuredSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := structuredResult(t, a, ctx, structuredOperation(), span)
			r.mu.Lock()
			held := r.payload
			r.mu.Unlock()
			switch mode {
			case "revoke":
				resolution.Grants[0].State = Revoked
			case "membership":
				missing = true
			case "source":
				span.facts.Connection.Fingerprint.EngineIdentity = "changed"
			case "replace":
				resolution.Grants[0].ID = "B"
				resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(time.Minute)
			case "cancel":
				cancel()
			case "expiry":
				<-r.purged
			}
			var output bytes.Buffer
			require.Error(t, r.WritePayload(context.Background(), &output))
			require.Zero(t, output.Len())
			assertResultPurged(t, r)
			require.Equal(t, make([]byte, len(held)), held)
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
		})
	}
}

func TestStructuredReadRealDeadlineClosesBlockedNativeRows(t *testing.T) {
	resolution := qualifiedResolution()
	resolution.ServerTime = time.Now()
	resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(100 * time.Millisecond)
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
	span := structuredSpan(&previewAdapter{reader: reader})
	a := testAuthority(t, fixedResolver(resolution))
	op := structuredOperation()
	request := structuredRequest(t, op)
	done := make(chan error, 1)
	go func() {
		r, err := a.ReadStructuredResult(context.Background(), request, op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { return span, nil }))
		if r != nil {
			r.Close()
		}
		done <- err
	}()
	select {
	case <-entered:
	case err := <-done:
		t.Fatalf("Next not entered: %v", err)
	case <-time.After(time.Second):
		t.Fatal("Next not entered")
	}
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("native lifetime not bounded")
	}
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
}

func TestStructuredIdentityInputCannotMutateRetainedIntent(t *testing.T) {
	span := structuredSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}})
	span.verify = func(_ context.Context, request StructuredIdentityRequest) error {
		request.Resources[0].Table.Name = "tampered"
		return nil
	}
	op := structuredOperation()
	source := structuredSourceFunc(func(_ context.Context, request StructuredIdentityRequest) (StructuredReadSpan, error) {
		request.Resources[0].Table.Name = "tampered"
		return span, nil
	})
	r, err := testAuthority(t, fixedResolver(qualifiedResolution())).ReadStructuredResult(context.Background(), structuredRequest(t, op), op, source)
	require.NoError(t, err)
	defer r.Close()
	require.True(t, slices.Equal(span.facts.Resources, []StructuredResourceIdentity{{Table: access.TableName{Schema: "sales", Name: "Customer"}}}))
}

func TestStructuredReadMemoryExecutesFilterProjectionJoinAggregateOrderAndLimit(t *testing.T) {
	newMap := func() *map[string]any { value := map[string]any{}; return &value }
	db := dalgo2memory.New(dalgo2memory.SingleWriterProfile(), dalgo2memory.WithSchema(false,
		dalgo2memory.WithCollection("Customer", newMap), dalgo2memory.WithCollection("Order", newMap)))
	writer := db.(dal.WriteSession)
	for _, item := range []struct {
		collection, id string
		data           map[string]any
	}{
		{"Customer", "1", map[string]any{"cid": int64(1), "name": "Ada"}},
		{"Customer", "2", map[string]any{"cid": int64(2), "name": "Grace"}},
		{"Order", "1", map[string]any{"customer": int64(1), "total": int64(4)}},
		{"Order", "2", map[string]any{"customer": int64(2), "total": int64(8)}},
		{"Order", "3", map[string]any{"customer": int64(2), "total": int64(16)}},
	} {
		require.NoError(t, writer.Insert(context.Background(), record.NewRecordWithData(record.NewKeyWithID(item.collection, item.id), &item.data)))
	}
	queries := []struct {
		text, parameters, rows string
		resources              []StructuredResourceIdentity
	}{
		{"from: {name: Customer}\nwhere: {op: '>=', left: {field: cid}, right: {param: min}}\ncolumns: [{field: name}]\norderBy: [{field: cid, desc: true}]\nlimit: 1", `{"min":{"type":"integer","value":"1"}}`, `[["Grace"]]`, []StructuredResourceIdentity{{Path: "/Customer"}}},
		{"from:\n  name: Customer\n  alias: c\n  joins:\n    - type: inner\n      from: {name: Order, alias: o}\n      on: [{op: '==', left: {field: cid, source: c}, right: {field: customer, source: o}}]\ncolumns: [{field: name, source: c, as: customer}, {field: total, source: o, as: total}]\norderBy: [{field: total, source: o, desc: true}]\nlimit: 2", "", `[["Grace",16],["Grace",8]]`, []StructuredResourceIdentity{{Path: "/Customer"}, {Path: "/Order"}}},
		{"from: {name: Order}\ncolumns: [{aggregate: {function: count, args: [{star: true}]}, as: count}, {aggregate: {function: sum, args: [{field: total}]}, as: total}]", "", `[[3,28]]`, []StructuredResourceIdentity{{Path: "/Order"}}},
	}
	for _, query := range queries {
		op := structuredOperation()
		op.TugQL = query.text
		op.Parameters = query.parameters
		span := structuredSpan(db)
		span.facts.Resources = query.resources
		r := structuredResult(t, testAuthority(t, fixedResolver(fixtureResolution())), context.Background(), op, span)
		var out bytes.Buffer
		require.NoError(t, r.WritePayload(context.Background(), &out))
		require.Contains(t, out.String(), `"rows":`+query.rows)
	}
}

func TestStructuredReadSubordinateRowAndFieldPoliciesRemainRestrictive(t *testing.T) {
	newMap := func() *map[string]any { v := map[string]any{}; return &v }
	db := dalgo2memory.New(dalgo2memory.SingleWriterProfile(), dalgo2memory.WithSchema(false, dalgo2memory.WithCollection("Customer", newMap)))
	writer := db.(dal.WriteSession)
	for _, item := range []struct {
		id, name string
		cid      int64
	}{{"one", "Ada", 1}, {"two", "Grace", 2}} {
		value := map[string]any{"cid": item.cid, "name": item.name, "private": "do not disclose"}
		require.NoError(t, writer.Insert(context.Background(), record.NewRecordWithData(record.NewKeyWithID("Customer", item.id), &value)))
	}
	policy, err := access.NewPolicyForRealm("sneat", "restricted", access.Root(access.Allow(access.Read).Fields("name").Where(dal.WhereField("cid", dal.Equal, int64(1)))))
	require.NoError(t, err)
	op := structuredOperation()
	op.TugQL = "from: {name: Customer}\ncolumns: [{field: name}]"
	span := structuredSpan(db)
	span.facts.Resources = []StructuredResourceIdentity{{Path: "/Customer"}}
	ctx := access.WithPolicy(context.Background(), policy)
	r := structuredResult(t, testAuthority(t, fixedResolver(fixtureResolution())), ctx, op, span)
	var output bytes.Buffer
	require.NoError(t, r.WritePayload(context.Background(), &output))
	require.Contains(t, output.String(), `"rows":[["Ada"]]`)
	require.NotContains(t, output.String(), "Grace")
	require.NotContains(t, output.String(), "do not disclose")
	require.Contains(t, r.executedTugQL, "where:")
	op.TugQL = "from: {name: Customer}\ncolumns: [{field: private}]"
	_, err = testAuthority(t, fixedResolver(fixtureResolution())).ReadStructuredResult(ctx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
		return structuredSpan(db), nil
	}))
	require.Error(t, err)
}

func TestStructuredReadExpiryPurgesThroughHeldResolverAndFactPorts(t *testing.T) {
	for _, stage := range []string{"resolver", "fact", "opener"} {
		t.Run(stage, func(t *testing.T) {
			resolution := qualifiedResolution()
			resolution.ServerTime = time.Now()
			resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
			resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(150 * time.Millisecond)
			stall := atomic.Bool{}
			entered := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			wait := func() { once.Do(func() { close(entered) }); <-release }
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				if stage == "resolver" && stall.Load() {
					wait()
				}
				return resolution, nil
			})
			adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}
			span := structuredSpan(adapter)
			span.verify = func(context.Context, StructuredIdentityRequest) error {
				if stage == "fact" && stall.Load() {
					wait()
				}
				return nil
			}
			op := structuredOperation()
			request := structuredRequest(t, op)
			if stage == "opener" {
				done := make(chan error, 1)
				go func() {
					r, err := a.ReadStructuredResult(context.Background(), request, op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { wait(); return span, nil }))
					if r != nil {
						r.Close()
					}
					done <- err
				}()
				<-entered
				time.Sleep(180 * time.Millisecond)
				close(release)
				require.Error(t, <-done)
				require.Zero(t, adapter.calls.Load())
				require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
				return
			}
			r := structuredResult(t, a, context.Background(), op, span)
			r.mu.Lock()
			held := r.payload
			r.mu.Unlock()
			stall.Store(true)
			var output bytes.Buffer
			done := make(chan error, 1)
			go func() { done <- r.WritePayload(context.Background(), &output) }()
			<-entered
			select {
			case <-r.purged:
			case <-time.After(time.Second):
				t.Fatal("held port prevented purge")
			}
			require.Equal(t, make([]byte, len(held)), held)
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
			close(release)
			require.Error(t, <-done)
			require.Zero(t, output.Len())
		})
	}
}

func TestStructuredNativeDispatchRejectsChangedOpaqueOrUnboundedRewrite(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	op := structuredOperation()
	q, _, err := op.query()
	require.NoError(t, err)
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
	span := structuredSpan(adapter)
	r := structuredResult(t, a, context.Background(), op, span)
	session := frozenStructuredSession{ReadSession: adapter, result: r, span: span}
	adapter.calls.Store(0)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), nil)
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), q, nil)
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), unencodableStructuredQuery{q})
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), largeStructuredQuery{q})
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), negativeLimitStructuredQuery{q})
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), invalidWhereStructuredQuery{q})
	require.Error(t, err)
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), &mutableLimitStructuredQuery{StructuredQuery: q})
	require.Error(t, err)
	for _, text := range []string{
		"from: {query: {as: nested, from: {schema: sales, name: Customer}, columns: [{field: id}]}}\ncolumns: [{field: id, source: nested}]",
		"from: {schema: sales, name: Customer}\ncolumns: [{query: {as: nested, from: {schema: sales, name: Customer}, columns: [{field: id}]}, as: nested_id}]",
	} {
		// This is a valid policy-effective query over an already-authorized
		// resource. Its recursive execution lifetime is still unproved: do not
		// let a complete resource mapping substitute for profile admission.
		rewritten, parseErr := tugql.Deserialize([]byte(text))
		require.NoError(t, parseErr)
		_, err = session.ExecuteQueryToRecordsetReader(context.Background(), rewritten)
		require.ErrorContains(t, err, "separately owned")
		require.Zero(t, adapter.calls.Load())
	}
	hidden := dal.From(dal.NewQualifiedRootCollectionRef("sales", "Hidden", "")).NewQuery().SelectIntoRecordset()
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), hidden)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load(), "policy-added inaccessible sources cannot reach native execution")
	_, err = a.executeStructuredReader(context.Background(), &lease{deadline: time.Now().Add(-time.Second)}, adapter, q)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
}

type largeStructuredQuery struct{ dal.StructuredQuery }

func (q largeStructuredQuery) Columns() []dal.Column {
	return []dal.Column{{Expression: dal.Constant{Value: strings.Repeat("x", 131073)}, Alias: "x"}}
}

type negativeLimitStructuredQuery struct{ dal.StructuredQuery }

func (q negativeLimitStructuredQuery) Limit() int { return -1 }

type invalidWhereStructuredQuery struct{ dal.StructuredQuery }

func (q invalidWhereStructuredQuery) Where() dal.Condition {
	return dal.Comparison{Operator: dal.Operator("unknown"), Left: dal.Field("id"), Right: dal.Constant{Value: int64(1)}}
}

type mutableLimitStructuredQuery struct {
	dal.StructuredQuery
	calls int
}

func (q *mutableLimitStructuredQuery) Limit() int {
	q.calls++
	if q.calls == 1 {
		return 0
	}
	return -1
}

func TestStructuredCancellationAtNativeOpenAndReaderCloseDiscardsEverything(t *testing.T) {
	for _, stage := range []string{"open", "close"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader := &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}
			if stage == "close" {
				reader.closeHook = func() error { cancel(); return nil }
			}
			adapter := previewFunctionAdapter{execute: func(dal.Query) (dal.RecordsetReader, error) {
				if stage == "open" {
					cancel()
				}
				return reader, nil
			}}
			span := structuredSpan(adapter)
			op := structuredOperation()
			r, err := testAuthority(t, fixedResolver(qualifiedResolution())).ReadStructuredResult(ctx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) { return span, nil }))
			require.Error(t, err)
			require.Nil(t, r)
			require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
			require.Eventually(t, func() bool { return reader.closes.Load() == 1 }, time.Second, time.Millisecond)
		})
	}
}

package dataaccess

import (
	"bytes"
	"context"
	"fmt"
	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExactTableAndStructuredTransportRecheckSamePolicyAndPurgeOldA(t *testing.T) {
	r := exactProjected(t, exactFixtureDocument())
	g := fixtureGrant(r.ServerTime)
	g.Scope.Objects = []ObjectRef{{Namespace: "sales", Name: "Order"}}
	g.Scope.Requires = []string{ObjectsExact}
	g.Reason = "read authority"
	g.Decision = nil
	r.Grants = []Grant{g}
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return r, nil })
	started := time.Now()
	baseline := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}))
	var wire bytes.Buffer
	require.NoError(t, baseline.WriteEnvelope(context.Background(), &wire))
	tableEnvelope, err := DecodeTableResultEnvelope(wire.Bytes())
	require.NoError(t, err)
	imported, err := a.ImportTableResult(context.Background(), resultRequest(), started, previewOperation(), tableEnvelope)
	require.NoError(t, err)
	defer imported.Close()
	op := structuredOperation()
	op.TugQL = "from: {schema: sales, name: Customer, alias: a, joins: [{from: {schema: sales, name: Order, alias: b}, on: [{left: {field: id, source: a}, op: '==', right: {field: id, source: b}}]}]}\ncolumns: [{field: id, source: a}]\nlimit: 2"
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}
	span := structuredSpan(adapter)
	span.facts.Resources = append(span.facts.Resources, StructuredResourceIdentity{Table: access.TableName{Schema: "sales", Name: "Order"}})
	started = time.Now()
	mixed := structuredResult(t, a, context.Background(), op, span)
	wire.Reset()
	require.NoError(t, mixed.WriteEnvelope(context.Background(), &wire))
	envelope, err := DecodeStructuredResultEnvelope(wire.Bytes())
	require.NoError(t, err)
	client, err := a.ImportStructuredResult(context.Background(), structuredRequest(t, op), started, op, envelope)
	require.NoError(t, err)
	defer client.Close()
	wire.Reset()
	require.NoError(t, client.WritePayload(context.Background(), &wire))
	require.Contains(t, wire.String(), "[[1]]")
	r.Grants[0].State = Revoked
	for _, retained := range []*Result{baseline, imported, mixed, client} {
		wire.Reset()
		require.Error(t, retained.WritePayload(context.Background(), &wire))
		require.Zero(t, wire.Len())
	}
	fresh := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(2)})}}))
	wire.Reset()
	require.NoError(t, fresh.WritePayload(context.Background(), &wire))
	require.Contains(t, wire.String(), "[[2]]")
}

func TestExactEffectiveSourceUnionStillDeniesCBeforeDataDispatch(t *testing.T) {
	a := testAuthority(t, fixedResolver(exactProjected(t, exactFixtureDocument())))
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
	span := structuredSpan(adapter)
	result := structuredResult(t, a, context.Background(), structuredOperation(), span)
	adapter.calls.Store(0)
	changed := dal.From(dal.NewQualifiedRootCollectionRef("sales", "Secret", "")).NewQuery().SelectIntoRecordset()
	_, err := (frozenStructuredSession{ReadSession: adapter, result: result, span: span}).ExecuteQueryToRecordsetReader(context.Background(), changed)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
}

func TestExactPrivateCChangePurgesHeldServerAndImportedMixedRead(t *testing.T) {
	raw := exactRawResolution()
	g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
	g.ExpiresAt = raw.ServerTime.Add(20 * time.Second)
	raw.Grants = []Grant{g}
	doc := exactFixtureDocument()
	a := testAuthority(t, func(ctx context.Context, _ ResolveRequest) (Resolution, error) {
		return ProjectExactRead(ctx, raw, exactSnapshot(t, doc))
	})
	baseline := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}))
	op := structuredOperation()
	op.TugQL = "from: {schema: sales, name: Customer, alias: a, joins: [{from: {schema: sales, name: Order, alias: b}, on: [{left: {field: id, source: a}, op: '==', right: {field: id, source: b}}]}]}\ncolumns: [{field: id, source: a}]\nlimit: 2"
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}
	span := structuredSpan(adapter)
	span.facts.Resources = append(span.facts.Resources, StructuredResourceIdentity{Table: access.TableName{Schema: "sales", Name: "Order"}})
	started := time.Now()
	server := structuredResult(t, a, context.Background(), op, span)
	var wire bytes.Buffer
	require.NoError(t, server.WriteEnvelope(context.Background(), &wire))
	envelope, err := DecodeStructuredResultEnvelope(wire.Bytes())
	require.NoError(t, err)
	client, err := a.ImportStructuredResult(context.Background(), structuredRequest(t, op), started, op, envelope)
	require.NoError(t, err)
	defer client.Close()
	// Only private C changes; the stored grant and coarse protection remain intact.
	doc.Generation++
	doc.Denials = append(doc.Denials, exactReadDeny{Path: "/Order"})
	require.Equal(t, Approved, raw.Grants[0].State)
	for _, held := range []*Result{baseline, server, client} {
		wire.Reset()
		require.Error(t, held.WritePayload(context.Background(), &wire))
		require.Zero(t, wire.Len())
	}
	fresh := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(2)})}}))
	wire.Reset()
	require.NoError(t, fresh.WritePayload(context.Background(), &wire))
	require.Contains(t, wire.String(), "[[2]]")
	_, err = a.authorizeRequest(withExactReadProfile(context.Background()), operationRequest(operationQuery("sales", "Order", "")))
	require.Error(t, err)
	_, err = a.ReadStructuredResult(context.Background(), structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
		t.Fatal("C-denied mixed operation opened source")
		return nil, nil
	}))
	require.Error(t, err)
}

func TestExactSameSnapshotBindsMandatoryNamespaceDuringActualRead(t *testing.T) {
	snapshot := exactSnapshot(t, exactFixtureDocument())
	raw := exactRawResolution()
	a := testAuthority(t, func(ctx context.Context, _ ResolveRequest) (Resolution, error) {
		return ProjectExactRead(ctx, raw, snapshot)
	})
	ctx, err := snapshot.BindContext(context.Background(), raw.Subject)
	require.NoError(t, err)
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(3)})}}
	result := previewResult(t, a, ctx, previewOperation(), newPreviewSpan(adapter))
	var output bytes.Buffer
	require.NoError(t, result.WritePayload(ctx, &output))
	require.Contains(t, output.String(), "[[3]]")
	require.EqualValues(t, 1, adapter.calls.Load())
}

func TestExactFrozenTableDispatchChecksFinalDeadlineAndPurge(t *testing.T) {
	a := testAuthority(t, fixedResolver(exactProjected(t, exactFixtureDocument())))
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
	result := previewResult(t, a, context.Background(), previewOperation(), newPreviewSpan(adapter))
	q, _, err := previewOperation().query()
	require.NoError(t, err)
	adapter.calls.Store(0)
	a.now = func() time.Time { return time.Now().Add(time.Hour) }
	session := frozenTableSession{ReadSession: adapter, query: q, result: result}
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), q)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
	result.Close()
	_, err = session.ExecuteQueryToRecordsetReader(context.Background(), q)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
}

// Deliberately keep the public resolver permissive: only the separately bound
// mandatory layer can block these actual secured/native dispatch attempts.
func TestExactMandatoryDenialsStopActualNativeDispatch(t *testing.T) {
	for _, name := range []string{"object", "path", "root", "agent"} {
		t.Run(name, func(t *testing.T) {
			doc := exactFixtureDocument()
			r := exactProjected(t, doc)
			subject := r.Subject
			if name == "agent" {
				subject = Subject{Realm: subject.Realm, Kind: Agent, ID: "demo-agent"}
				var err error
				r, err = ProjectAgentExactRead(context.Background(), exactRawResolution(), subject, exactSnapshot(t, doc))
				require.NoError(t, err)
			}
			a, err := NewAuthority(subject, r.Connection, fixedResolver(r))
			require.NoError(t, err)
			deny := exactReadDeny{Object: &ObjectRef{"sales", "Customer"}}
			switch name {
			case "path":
				deny = exactReadDeny{Path: "/Customer"}
			case "root":
				deny = exactReadDeny{Path: "/"}
			case "agent":
				deny.Subject = &subject
			}
			for _, structured := range []bool{false, true} {
				t.Run(fmt.Sprintf("structured=%t", structured), func(t *testing.T) {
					for _, denied := range []bool{false, true} {
						configured := doc
						if denied {
							configured.Denials = append(append([]exactReadDeny{}, doc.Denials...), deny)
						}
						ctx, err := exactSnapshot(t, configured).BindContext(context.Background(), subject)
						require.NoError(t, err)
						adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
						var result *Result
						if structured {
							op := structuredOperation()
							result, err = a.ReadStructuredResult(ctx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
								return structuredSpan(adapter), nil
							}))
						} else {
							result, err = a.ReadTableResult(ctx, resultRequest(), previewOperation(), tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) {
								return newPreviewSpan(adapter), nil
							}))
						}
						if !denied {
							require.NoError(t, err)
							require.EqualValues(t, 1, adapter.calls.Load())
							result.Close()
							continue
						}
						var acl *Denial
						require.ErrorAs(t, err, &acl)
						require.Equal(t, DataRead, acl.Capability)
						if structured {
							require.Equal(t, "STRUCTURED_READ_FAILED", acl.Code)
						} else {
							require.Equal(t, "TABLE_READ_FAILED", acl.Code)
						}
						require.Zero(t, adapter.calls.Load())
						// Inspect the actual secured-session error before Core intentionally hides
						// private policy details behind its typed public denial.
						q, req, err := previewOperation().query()
						require.NoError(t, err)
						prepared, lease, cancel, err := a.prepareRequest(withExactReadProfile(ctx), nil, req)
						require.NoError(t, err)
						_, err = access.SecureReadSession(adapter, lease.policy).ExecuteQueryToRecordsetReader(prepared, q)
						cancel()
						require.ErrorIs(t, err, access.ErrAccessDenied)
						var nativeDenial *access.DeniedError
						require.ErrorAs(t, err, &nativeDenial)
						require.Equal(t, "datatug-exact-mandatory", nativeDenial.Decision.Policy)
						require.Zero(t, adapter.calls.Load())
					}
				})
			}
		})
	}
}

func TestExactSecuredSessionRejectsExpiredPreparedContextBeforeNativeDispatch(t *testing.T) {
	r := exactProjected(t, exactFixtureDocument())
	r.ValidUntil = r.ServerTime.Add(time.Second)
	a := testAuthority(t, fixedResolver(r))
	q, req, err := previewOperation().query()
	require.NoError(t, err)
	// Use Core-owned authority expiry, with no caller timeout to hide a missing guard.
	prepared, lease, cancel, err := a.prepareRequest(withExactReadProfile(context.Background()), nil, req)
	require.NoError(t, err)
	defer cancel()
	adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
	secured := access.SecureReadSession(adapter, lease.policy)
	positive, err := secured.ExecuteQueryToRecordsetReader(prepared, q)
	require.NoError(t, err)
	require.EqualValues(t, 1, adapter.calls.Load())
	require.NoError(t, positive.Close())
	adapter.calls.Store(0)

	// A caller cannot detach a retained policy's deadline and reuse its values.
	_, err = secured.ExecuteQueryToRecordsetReader(context.WithoutCancel(prepared), q)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
	<-prepared.Done()
	_, err = secured.ExecuteQueryToRecordsetReader(prepared, q)
	require.Error(t, err)
	require.Zero(t, adapter.calls.Load())
}

func TestExactBoundSubjectMismatchAndSourceProfileStripping(t *testing.T) {
	s := exactSnapshot(t, exactFixtureDocument())
	r := exactProjected(t, exactFixtureDocument())
	a := testAuthority(t, fixedResolver(r))
	wrong := r.Subject
	wrong.ID = "other"
	ctx, err := s.BindContext(context.Background(), wrong)
	require.NoError(t, err)
	source := tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) {
		t.Fatal("mismatched source opened")
		return nil, nil
	})
	_, err = a.ReadTableResult(ctx, resultRequest(), previewOperation(), source)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	var mismatch *Denial
	require.ErrorAs(t, err, &mismatch)
	require.Equal(t, "EXACT_BINDING_MISMATCH", mismatch.Code)
	op := structuredOperation()
	otherDoc := exactFixtureDocument()
	otherDoc.Connection.ID = "other-connection"
	otherCtx, bindErr := exactSnapshot(t, otherDoc).BindContext(context.Background(), r.Subject)
	require.NoError(t, bindErr)
	_, err = a.ReadTableResult(otherCtx, resultRequest(), previewOperation(), source)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.ErrorAs(t, err, &mismatch)
	require.Equal(t, "EXACT_BINDING_MISMATCH", mismatch.Code)
	_, err = a.ReadStructuredResult(otherCtx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
		t.Fatal("wrong-connection source opened")
		return nil, nil
	}))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.ErrorAs(t, err, &mismatch)
	require.Equal(t, "EXACT_BINDING_MISMATCH", mismatch.Code)
	_, err = a.ReadStructuredResult(ctx, structuredRequest(t, op), op, structuredSourceFunc(func(context.Context, StructuredIdentityRequest) (StructuredReadSpan, error) {
		t.Fatal("mismatched source opened")
		return nil, nil
	}))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.ErrorAs(t, err, &mismatch)
	require.Equal(t, "EXACT_BINDING_MISMATCH", mismatch.Code)
	for _, structured := range []bool{false, true} {
		adapter := &previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}}
		verificationCalls, nativeCalls := 0, 0
		var result *Result
		if structured {
			result, err = a.ReadStructuredResult(context.Background(), structuredRequest(t, op), op, structuredSourceFunc(func(ctx context.Context, _ StructuredIdentityRequest) (StructuredReadSpan, error) {
				require.Nil(t, ctx.Value(exactReadProfileKey{}))
				span := structuredSpan(adapter)
				span.session = exactMarkerCheckingSession{ReadSession: adapter, t: t, calls: &nativeCalls}
				span.verify = func(ctx context.Context, _ StructuredIdentityRequest) error {
					verificationCalls++
					require.Nil(t, ctx.Value(exactReadProfileKey{}))
					return nil
				}
				return span, nil
			}))
		} else {
			result, err = a.ReadTableResult(context.Background(), resultRequest(), previewOperation(), tableSourceFunc(func(ctx context.Context, _ TableIdentityRequest) (TableReadSpan, error) {
				require.Nil(t, ctx.Value(exactReadProfileKey{}))
				span := newPreviewSpan(adapter)
				span.session = exactMarkerCheckingSession{ReadSession: adapter, t: t, calls: &nativeCalls}
				span.verify = func(ctx context.Context) error {
					verificationCalls++
					require.Nil(t, ctx.Value(exactReadProfileKey{}))
					return nil
				}
				return span, nil
			}))
		}
		require.NoError(t, err)
		defer result.Close()
		// Empty result: pre-read, post-read, pre/post Next and final guard.
		// Structured additionally verifies the effective query before dispatch.
		expectedVerifications := 5
		if structured {
			expectedVerifications++
		}
		require.Equal(t, expectedVerifications, verificationCalls)
		require.Equal(t, 1, nativeCalls)
		result.Close()
	}
}

func TestExactSecuredSessionRejectsContextLongerThanSelectedGrant(t *testing.T) {
	raw := exactRawResolution()
	g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
	g.ExpiresAt = raw.ServerTime.Add(5 * time.Second)
	raw.Grants = []Grant{g}
	r, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, exactFixtureDocument()))
	require.NoError(t, err)
	p := newOperationReadPolicy(r, r.ServerTime, time.Now)
	q := operationQuery("sales", "Order", "")
	ctx, cancel := context.WithDeadline(context.Background(), p.ceiling)
	defer cancel()
	adapter := &recordingAdapter{}
	_, err = access.SecureReadSession(adapter, p).ExecuteQueryToRecordsReader(ctx, q)
	require.Error(t, err)
	require.Empty(t, adapter.calls)
	bounded, stop := context.WithDeadline(context.Background(), g.ExpiresAt)
	defer stop()
	require.True(t, p.Decide(bounded, *operationRequest(q)).Allowed)
}

// Legacy point profiles expose prepared contexts to DALgo. They obey the same
// pure bounded-context contract even without a table/structured final wrapper.
func TestExactPureDeadlineGuardAlsoProtectsLegacyRawPointSession(t *testing.T) {
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		t.Run(string(posture), func(t *testing.T) {
			r := fixtureResolution()
			r.Protection.ReadPosture = posture
			r.ValidUntil = r.ServerTime.Add(time.Second)
			if posture == ReadNone {
				g := fixtureGrant(r.ServerTime)
				g.ExpiresAt = r.ServerTime.Add(time.Second)
				r.Grants = []Grant{g}
			}
			a := testAuthority(t, fixedResolver(r))
			prepared, lease, cancel, err := a.prepare(context.Background(), nil)
			require.NoError(t, err)
			defer cancel()
			adapter := &recordingAdapter{}
			session := access.SecureReadSession(adapter, lease.policy)
			key := record.NewKeyWithID("docs", "one")
			_, err = session.Exists(prepared, key)
			require.NoError(t, err)
			require.Len(t, adapter.calls, 1)
			adapter.calls = nil
			_, err = session.Exists(context.WithoutCancel(prepared), key)
			require.Error(t, err)
			require.Empty(t, adapter.calls)
			<-prepared.Done()
			_, err = session.Exists(prepared, key)
			require.Error(t, err)
			require.Empty(t, adapter.calls)
		})
	}
}

type exactMarkerCheckingSession struct {
	dal.ReadSession
	t     *testing.T
	calls *int
}

func (s exactMarkerCheckingSession) ExecuteQueryToRecordsetReader(ctx context.Context, q dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	*s.calls++
	require.Nil(s.t, ctx.Value(exactReadProfileKey{}))
	return s.ReadSession.ExecuteQueryToRecordsetReader(ctx, q, options...)
}

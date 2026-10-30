package dataaccess

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/record"
	"github.com/dal-go/record/update"
	"github.com/stretchr/testify/require"
)

func fixtureSubject() Subject { return Subject{Realm: "sneat", Kind: User, ID: "sam"} }
func fixtureConnection() ConnectionRef {
	return ConnectionRef{Space: "space-a", ID: "conn", Fingerprint: Fingerprint{Engine: "recording", Database: "orders", EngineIdentity: "cluster-a", Address: "hint"}}
}
func fixtureGrant(now time.Time) Grant {
	return Grant{ID: "grant-1", Subject: fixtureSubject(), RequestedBy: fixtureSubject(), Permissions: []Permission{DataRead},
		Scope: GrantScope{Connection: "conn", Requires: []string{}}, Reason: "Investigate incident", ExpiresAt: now.Add(time.Minute),
		State: Approved, Decision: &Decision{By: Subject{Realm: "sneat", Kind: User, ID: "priya"}, At: now}, Revision: 1}
}
func fixtureResolution() Resolution {
	now := time.Now()
	connection := fixtureConnection()
	return Resolution{Subject: fixtureSubject(), Connection: connection, ServerTime: now, ValidUntil: now.Add(time.Minute),
		Protection: ConnectionProtection{ID: connection.ID, Fingerprint: connection.Fingerprint, ReadOnly: true, ReadPosture: ReadAll, AgentRead: AgentInherit, Revision: 1}}
}
func fixedResolver(resolution Resolution) Resolver {
	return func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil }
}
func testAuthority(t *testing.T, resolver Resolver) *Authority {
	t.Helper()
	a, err := NewAuthority(fixtureSubject(), fixtureConnection(), resolver)
	require.NoError(t, err)
	return a
}
func testDatabase(t *testing.T, authority *Authority, backend dal.DB) Database {
	t.Helper()
	db, err := authority.SecureDB(backend)
	require.NoError(t, err)
	return db
}
func keyQuery() dal.StructuredQuery {
	return dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().SelectKeysOnly(reflect.String)
}

// recordingAdapter implements its own DALgo adapter. Every entry, including a
// write that would succeed, records a dispatch. It has no approval logic.
type recordingAdapter struct {
	dal.NoConcurrency
	calls  []string
	hook   func(context.Context)
	err    error
	reader dal.RecordsReader
}

func (a *recordingAdapter) ID() string           { return "recording-a" }
func (a *recordingAdapter) Adapter() dal.Adapter { return dal.NewAdapter("recording-a", "test") }
func (a *recordingAdapter) Schema() dal.Schema   { a.calls = append(a.calls, "Schema"); return nil }
func (a *recordingAdapter) call(ctx context.Context, name string) error {
	a.calls = append(a.calls, name)
	if a.hook != nil {
		a.hook(ctx)
	}
	return a.err
}
func (a *recordingAdapter) Exists(ctx context.Context, _ *record.Key) (bool, error) {
	return true, a.call(ctx, "Exists")
}
func (a *recordingAdapter) Get(ctx context.Context, rec record.Record) error {
	return a.GetMulti(ctx, []record.Record{rec})
}
func (a *recordingAdapter) GetMulti(ctx context.Context, records []record.Record) error {
	err := a.call(ctx, "GetMulti")
	for _, rec := range records {
		rec.SetError(err)
	}
	return err
}
func (a *recordingAdapter) ExecuteQueryToRecordsReader(ctx context.Context, _ dal.Query) (dal.RecordsReader, error) {
	if err := a.call(ctx, "Query"); err != nil {
		return nil, err
	}
	if a.reader == nil {
		a.reader = &testReader{}
	}
	return a.reader, nil
}
func (a *recordingAdapter) ExecuteQueryToRecordsetReader(ctx context.Context, _ dal.Query, _ ...recordset.Option) (dal.RecordsetReader, error) {
	return nil, a.call(ctx, "Recordset")
}
func (a *recordingAdapter) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	if err := a.call(ctx, "ReadonlyTransaction"); err != nil {
		return err
	}
	return worker(ctx, &recordingTx{ReadSession: a, options: dal.NewTransactionOptions(options...)})
}
func (a *recordingAdapter) RunReadwriteTransaction(ctx context.Context, _ dal.RWTxWorker, _ ...dal.TransactionOption) error {
	return a.call(ctx, "ReadwriteTransaction")
}
func (a *recordingAdapter) Set(ctx context.Context, _ record.Record) error { return a.call(ctx, "Set") }
func (a *recordingAdapter) SetMulti(ctx context.Context, _ []record.Record) error {
	return a.call(ctx, "SetMulti")
}
func (a *recordingAdapter) Insert(ctx context.Context, _ record.Record, _ ...dal.InsertOption) error {
	return a.call(ctx, "Insert")
}
func (a *recordingAdapter) InsertMulti(ctx context.Context, _ []record.Record, _ ...dal.InsertOption) error {
	return a.call(ctx, "InsertMulti")
}
func (a *recordingAdapter) Update(ctx context.Context, _ *record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return a.call(ctx, "Update")
}
func (a *recordingAdapter) UpdateRecord(ctx context.Context, _ record.Record, _ []update.Update, _ ...dal.Precondition) error {
	return a.call(ctx, "UpdateRecord")
}
func (a *recordingAdapter) UpdateMulti(ctx context.Context, _ []*record.Key, _ []update.Update, _ ...dal.Precondition) error {
	return a.call(ctx, "UpdateMulti")
}
func (a *recordingAdapter) Delete(ctx context.Context, _ *record.Key) error {
	return a.call(ctx, "Delete")
}
func (a *recordingAdapter) DeleteMulti(ctx context.Context, _ []*record.Key) error {
	return a.call(ctx, "DeleteMulti")
}

type recordingTx struct {
	dal.ReadSession
	options dal.TransactionOptions
}

func (tx *recordingTx) Options() dal.TransactionOptions { return tx.options }

// memoryRecorder wraps the real in-memory adapter with a separate recording
// implementation. Its successful underlying writes demonstrate that denial is
// supplied by shared core/DALgo, not by a read-only fake adapter.
type memoryRecorder struct {
	dal.DB
	calls  []string
	writer dal.WriteSession
}

func newMemoryRecorder(t *testing.T) *memoryRecorder {
	t.Helper()
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	writer := db.(dal.WriteSession)
	require.NoError(t, writer.Insert(context.Background(), record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"value": "fixture"})))
	return &memoryRecorder{DB: db, writer: writer}
}
func (m *memoryRecorder) Exists(ctx context.Context, key *record.Key) (bool, error) {
	m.calls = append(m.calls, "Exists")
	return m.DB.Exists(ctx, key)
}
func (m *memoryRecorder) Set(ctx context.Context, rec record.Record) error {
	m.calls = append(m.calls, "Set")
	return m.writer.Set(ctx, rec)
}
func (m *memoryRecorder) SetMulti(ctx context.Context, records []record.Record) error {
	m.calls = append(m.calls, "SetMulti")
	return m.writer.SetMulti(ctx, records)
}
func (m *memoryRecorder) Insert(ctx context.Context, rec record.Record, options ...dal.InsertOption) error {
	m.calls = append(m.calls, "Insert")
	return m.writer.Insert(ctx, rec, options...)
}
func (m *memoryRecorder) InsertMulti(ctx context.Context, records []record.Record, options ...dal.InsertOption) error {
	m.calls = append(m.calls, "InsertMulti")
	return m.writer.InsertMulti(ctx, records, options...)
}
func (m *memoryRecorder) Update(ctx context.Context, key *record.Key, changes []update.Update, preconditions ...dal.Precondition) error {
	m.calls = append(m.calls, "Update")
	return m.writer.Update(ctx, key, changes, preconditions...)
}
func (m *memoryRecorder) UpdateRecord(ctx context.Context, rec record.Record, changes []update.Update, preconditions ...dal.Precondition) error {
	m.calls = append(m.calls, "UpdateRecord")
	return m.writer.UpdateRecord(ctx, rec, changes, preconditions...)
}
func (m *memoryRecorder) UpdateMulti(ctx context.Context, keys []*record.Key, changes []update.Update, preconditions ...dal.Precondition) error {
	m.calls = append(m.calls, "UpdateMulti")
	return m.writer.UpdateMulti(ctx, keys, changes, preconditions...)
}
func (m *memoryRecorder) Delete(ctx context.Context, key *record.Key) error {
	m.calls = append(m.calls, "Delete")
	return m.writer.Delete(ctx, key)
}
func (m *memoryRecorder) DeleteMulti(ctx context.Context, keys []*record.Key) error {
	m.calls = append(m.calls, "DeleteMulti")
	return m.writer.DeleteMulti(ctx, keys)
}

type testReader struct {
	mu          sync.Mutex
	next        func() (record.Record, error)
	cursor      func() (string, error)
	nextCalls   int
	cursorCalls int
	closed      chan struct{}
	closeCalls  int
	closeErr    error
}

func (r *testReader) Next() (record.Record, error) {
	r.mu.Lock()
	r.nextCalls++
	r.mu.Unlock()
	if r.next != nil {
		return r.next()
	}
	return record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"value": "fixture"}), nil
}
func (r *testReader) Cursor() (string, error) {
	r.mu.Lock()
	r.cursorCalls++
	r.mu.Unlock()
	if r.cursor != nil {
		return r.cursor()
	}
	return "private-cursor", nil
}
func (r *testReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closeCalls++
	if r.closed != nil && r.closeCalls == 1 {
		close(r.closed)
	}
	return r.closeErr
}

func writeVectors(ctx context.Context, writer dal.WriteSession) []func() error {
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"value": "change"})
	records := []record.Record{rec}
	changes := []update.Update{update.ByFieldName("value", "changed")}
	return []func() error{
		func() error { return writer.Set(ctx, rec) }, func() error { return writer.SetMulti(ctx, records) },
		func() error { return writer.Insert(ctx, rec) }, func() error { return writer.InsertMulti(ctx, records) },
		func() error { return writer.Update(ctx, rec.Key(), changes) }, func() error { return writer.UpdateRecord(ctx, rec, changes) },
		func() error { return writer.UpdateMulti(ctx, []*record.Key{rec.Key()}, changes) },
		func() error { return writer.Delete(ctx, rec.Key()) }, func() error { return writer.DeleteMulti(ctx, []*record.Key{rec.Key()}) },
	}
}

func TestCORE1SameDecisionsAndZeroWriteDispatchOnTwoAdapters(t *testing.T) {
	a := &recordingAdapter{}
	b := newMemoryRecorder(t)
	require.NoError(t, a.Set(context.Background(), record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{})))
	a.calls = nil
	type decisionVector struct {
		operation access.Operations
		code      access.ReasonCode
		resource  string
	}
	var expected []decisionVector
	for i, raw := range []dal.DB{dal.NewDB(a), b} {
		boundary := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), raw)
		// A caller can add a permissive policy, but cannot remove the core's
		// database policy or acquire a write rule from local ownership.
		ctx := access.WithPolicy(context.Background(), access.MustPolicy("caller-owner", access.Root(access.Allow(access.ReadWrite, "owner"))))
		var decisions []decisionVector
		for _, vector := range writeVectors(ctx, boundary) {
			err := vector()
			require.ErrorIs(t, err, access.ErrAccessDenied)
			decision := access.DecisionsFromError(err)[0]
			decisions = append(decisions, decisionVector{decision.Operation, decision.Code, decision.Resource.String()})
		}
		if i == 0 {
			expected = decisions
		} else {
			require.Equal(t, expected, decisions)
		}
		require.Empty(t, a.calls)
		require.Empty(t, b.calls)
		exists, err := boundary.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.NoError(t, err)
		require.True(t, exists)
		a.calls = nil
		b.calls = nil
	}
}

func TestCORE4AuthorityFailuresAndBindingNeverDispatch(t *testing.T) {
	for name, mutate := range map[string]func(*Resolution){
		"subject":                      func(r *Resolution) { r.Subject.ID = "other" },
		"subject kind":                 func(r *Resolution) { r.Subject.Kind = Agent },
		"subject realm":                func(r *Resolution) { r.Subject.Realm = "other" },
		"Space":                        func(r *Resolution) { r.Connection.Space = "other" },
		"connection":                   func(r *Resolution) { r.Connection.ID = "other" },
		"protection":                   func(r *Resolution) { r.Protection.ID = "other" },
		"engine":                       func(r *Resolution) { r.Connection.Fingerprint.Engine = "other" },
		"database":                     func(r *Resolution) { r.Protection.Fingerprint.Database = "other" },
		"identity":                     func(r *Resolution) { r.Connection.Fingerprint.EngineIdentity = "other" },
		"missing time":                 func(r *Resolution) { r.ServerTime = time.Time{} },
		"expired snapshot":             func(r *Resolution) { r.ValidUntil = r.ServerTime },
		"unknown posture":              func(r *Resolution) { r.Protection.ReadPosture = "" },
		"unknown agent posture":        func(r *Resolution) { r.Protection.AgentRead = "" },
		"unsupported mandatory policy": func(r *Resolution) { r.Protection.MandatoryFineGrained = true },
		"none":                         func(r *Resolution) { r.Protection.ReadPosture = ReadNone },
		"grant wrong subject":          func(r *Resolution) { r.Grants = []Grant{fixtureGrant(r.ServerTime)}; r.Grants[0].Subject.ID = "other" },
		"grant wrong connection": func(r *Resolution) {
			r.Grants = []Grant{fixtureGrant(r.ServerTime)}
			r.Grants[0].Scope.Connection = "other"
		},
		"unknown scope feature": func(r *Resolution) {
			r.Grants = []Grant{fixtureGrant(r.ServerTime)}
			r.Grants[0].Scope.Requires = []string{"future"}
		},
		"object execution gated": func(r *Resolution) {
			r.Grants = []Grant{fixtureGrant(r.ServerTime)}
			r.Grants[0].Scope.Requires = []string{ObjectsExact}
			r.Grants[0].Scope.Objects = []ObjectRef{{Name: "docs"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			r := fixtureResolution()
			mutate(&r)
			a := &recordingAdapter{}
			db := testDatabase(t, testAuthority(t, fixedResolver(r)), dal.NewDB(a))
			exists, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
			require.ErrorIs(t, err, access.ErrAccessDenied)
			require.False(t, exists)
			require.Empty(t, a.calls)
		})
	}
	a := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		return Resolution{}, errors.New("secret upstream token")
	}), dal.NewDB(a))
	_, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.NotContains(t, err.Error(), "secret")
	require.Empty(t, a.calls)
}

func TestAuthorityConstructionAndProviderAreClosed(t *testing.T) {
	_, err := NewAuthority(Subject{}, fixtureConnection(), fixedResolver(fixtureResolution()))
	require.Error(t, err)
	_, err = NewAuthority(fixtureSubject(), ConnectionRef{}, fixedResolver(fixtureResolution()))
	require.Error(t, err)
	_, err = NewAuthority(fixtureSubject(), fixtureConnection(), nil)
	require.Error(t, err)
	var absent *Authority
	_, err = absent.SecureDB(nil)
	require.Error(t, err)
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	_, err = a.SecureDB(nil)
	require.Error(t, err)
	_, err = policyForOperation(context.Background())
	require.Error(t, err)
	// The fallback writer still denies every method for a read-only adapter.
	raw := &readOnlyBackend{recordingAdapter: &recordingAdapter{}}
	db := testDatabase(t, a, dal.NewDB(raw))
	for _, vector := range writeVectors(context.Background(), db) {
		require.ErrorIs(t, vector(), access.ErrAccessDenied)
	}
	require.Empty(t, raw.recordingAdapter.calls)
	_, exposed := db.(dal.DB)
	require.False(t, exposed)
	_, exposed = db.(dal.ReadwriteTransactionCoordinator)
	require.False(t, exposed)
}

type readOnlyBackend struct {
	recordingAdapter *recordingAdapter
	dal.NoConcurrency
}

func (b *readOnlyBackend) ID() string           { return b.recordingAdapter.ID() }
func (b *readOnlyBackend) Adapter() dal.Adapter { return b.recordingAdapter.Adapter() }
func (b *readOnlyBackend) Schema() dal.Schema   { return b.recordingAdapter.Schema() }
func (b *readOnlyBackend) Exists(ctx context.Context, key *record.Key) (bool, error) {
	return b.recordingAdapter.Exists(ctx, key)
}
func (b *readOnlyBackend) Get(ctx context.Context, rec record.Record) error {
	return b.recordingAdapter.Get(ctx, rec)
}
func (b *readOnlyBackend) GetMulti(ctx context.Context, records []record.Record) error {
	return b.recordingAdapter.GetMulti(ctx, records)
}
func (b *readOnlyBackend) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	return b.recordingAdapter.ExecuteQueryToRecordsReader(ctx, query)
}
func (b *readOnlyBackend) ExecuteQueryToRecordsetReader(ctx context.Context, query dal.Query, options ...recordset.Option) (dal.RecordsetReader, error) {
	return b.recordingAdapter.ExecuteQueryToRecordsetReader(ctx, query, options...)
}
func (b *readOnlyBackend) RunReadonlyTransaction(ctx context.Context, worker dal.ROTxWorker, options ...dal.TransactionOption) error {
	return b.recordingAdapter.RunReadonlyTransaction(ctx, worker, options...)
}
func (b *readOnlyBackend) RunReadwriteTransaction(ctx context.Context, worker dal.RWTxWorker, options ...dal.TransactionOption) error {
	return b.recordingAdapter.RunReadwriteTransaction(ctx, worker, options...)
}

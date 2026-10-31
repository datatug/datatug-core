package dataaccess

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

type tableIdentityFunc func(context.Context, dal.DB, TableIdentityRequest) (TableIdentityFacts, error)

func (f tableIdentityFunc) VerifyTableIdentity(ctx context.Context, db dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
	return f(ctx, db, request)
}

// This is a closed synthetic handle registry, not native identifier evidence.
// Facts are held independently of request.Connection and request.Table; an
// unknown handle/name refuses rather than echoing caller-provided claims.
type identityFixture struct {
	db     dal.DB
	source ConnectionRef
	tables map[access.TableName]access.TableName
	calls  atomic.Int64
}

func newIdentityFixture(db dal.DB) *identityFixture {
	return &identityFixture{db: db, source: fixtureConnection(), tables: map[access.TableName]access.TableName{
		{Schema: "sales", Name: "Customer"}:      {Schema: "sales", Name: "Customer"},
		{Schema: "sales", Name: "Order"}:         {Schema: "sales", Name: "Order"},
		{Schema: `a.b`, Name: `"Customer".part`}: {Schema: `a.b`, Name: `"Customer".part`},
	}}
}
func (f *identityFixture) VerifyTableIdentity(_ context.Context, db dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
	f.calls.Add(1)
	if db != f.db {
		return TableIdentityFacts{}, errors.New("fixture: unknown actual handle")
	}
	table, ok := f.tables[request.Table]
	if !ok {
		return TableIdentityFacts{}, errors.New("fixture: unknown native name")
	}
	return TableIdentityFacts{Connection: f.source, Table: table}, nil
}
func qualifiedResolution() Resolution {
	r := fixtureResolution()
	r.Protection.ReadPosture = ReadNone
	a := fixtureGrant(r.ServerTime)
	a.ID, a.ExpiresAt = "a", r.ServerTime.Add(20*time.Second)
	a.Scope.Objects, a.Scope.Requires = []ObjectRef{{Namespace: "sales", Name: "Customer"}}, []string{ObjectsExact}
	b := fixtureGrant(r.ServerTime)
	b.ID = "b"
	b.Scope.Objects, b.Scope.Requires = []ObjectRef{{Namespace: "sales", Name: "Order"}}, []string{ObjectsExact}
	r.Grants = []Grant{a, b}
	return r
}
func identityDatabase(t *testing.T, a *Authority, raw dal.DB, verifier TableIdentityVerifier) Database {
	t.Helper()
	db, err := a.SecureDBWithTableIdentity(raw, verifier)
	require.NoError(t, err)
	return db
}

func TestTableIdentityDefaultMissingAndUnsupportedNeverDispatch(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	native := &recordingAdapter{}
	raw := dal.NewDB(native)
	_, err := a.SecureDBWithTableIdentity(raw, nil)
	require.Error(t, err)
	var typedNil *identityFixture
	_, err = a.SecureDBWithTableIdentity(raw, typedNil)
	require.Error(t, err)
	reader, err := testDatabase(t, a, raw).ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.Error(t, err)
	require.Nil(t, reader)
	facts := newIdentityFixture(raw)
	db := identityDatabase(t, a, raw, facts)
	for _, query := range []dal.Query{
		operationQuery("sales", "Other", ""), keyQuery(),
		dal.NewTextQuery("SELECT secret", nil),
		dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")).NewQuery().WhereField("secret", dal.Equal, "x").SelectKeysOnly(reflect.String),
		compositionJoin(dal.NewQualifiedRootCollectionRef("sales", "Customer", "c"), dal.NewQualifiedRootCollectionRef("sales", "Order", "o")),
		dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")).NewQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate("UNKNOWN", false, dal.Field("id"))}),
	} {
		reader, err = db.ExecuteQueryToRecordsReader(context.Background(), query)
		require.Error(t, err)
		require.Nil(t, reader)
	}
	_, err = db.Exists(context.Background(), record.NewKeyWithID("sales.Customer", "one"))
	require.Error(t, err)
	for _, write := range writeVectors(context.Background(), db) {
		require.Error(t, write())
	}
	require.Zero(t, facts.calls.Load())
	require.Empty(t, native.calls)
	// Function-valued verifiers are accepted as trusted host ports; typed nil
	// function implementations are refused without calling them.
	var nilFunc tableIdentityFunc
	_, err = a.SecureDBWithTableIdentity(raw, nilFunc)
	require.Error(t, err)
}

func TestTableIdentityAcceptsOnlyIndependentlyBoundLiteralFacts(t *testing.T) {
	for _, object := range []ObjectRef{{Namespace: "sales", Name: "Customer"}, {Namespace: `a.b`, Name: `"Customer".part`}} {
		r := qualifiedResolution()
		r.Grants[0].Scope.Objects[0] = object
		a := testAuthority(t, fixedResolver(r))
		native := &recordingAdapter{}
		raw := dal.NewDB(native)
		facts := newIdentityFixture(raw)
		db := identityDatabase(t, a, raw, facts)
		query := dal.From(dal.NewQualifiedRootCollectionRef(object.Namespace, object.Name, "alias")).NewQuery().Limit(7).Offset(3).StartFrom("cursor").SelectKeysOnly(reflect.String)
		reader, err := db.ExecuteQueryToRecordsReader(context.Background(), query)
		require.NoError(t, err)
		require.Equal(t, []string{"Query"}, native.calls)
		require.EqualValues(t, 2, facts.calls.Load(), "initial facts and post-I/O facts")
		rec, err := reader.Next()
		require.NoError(t, err)
		require.NotNil(t, rec)
		cursor, err := reader.Cursor()
		require.NoError(t, err)
		require.NotEmpty(t, cursor)
		require.NoError(t, reader.Close())
	}
}
func TestTableIdentityRejectsMismatchedFactsBeforeData(t *testing.T) {
	for name, mutate := range map[string]func(*TableIdentityFacts){
		"space":              func(f *TableIdentityFacts) { f.Connection.Space = "other" },
		"connection":         func(f *TableIdentityFacts) { f.Connection.ID = "other" },
		"engine":             func(f *TableIdentityFacts) { f.Connection.Fingerprint.Engine = "other" },
		"database":           func(f *TableIdentityFacts) { f.Connection.Fingerprint.Database = "other" },
		"native source":      func(f *TableIdentityFacts) { f.Connection.Fingerprint.EngineIdentity = "other" },
		"folded name":        func(f *TableIdentityFacts) { f.Table.Name = "customer" },
		"guessed namespace":  func(f *TableIdentityFacts) { f.Table.Schema = "public" },
		"database component": func(f *TableIdentityFacts) { f.Table.Database = "app" },
		"dot rewrite":        func(f *TableIdentityFacts) { f.Table.Name = "sales.Customer" },
		"quote rewrite":      func(f *TableIdentityFacts) { f.Table.Name = `"Customer"` },
	} {
		t.Run(name, func(t *testing.T) {
			native := &recordingAdapter{}
			raw := dal.NewDB(native)
			fixture := newIdentityFixture(raw)
			verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
				f, err := fixture.VerifyTableIdentity(ctx, actual, request)
				require.NoError(t, err)
				mutate(&f)
				return f, nil
			})
			db := identityDatabase(t, testAuthority(t, fixedResolver(qualifiedResolution())), raw, verifier)
			reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
			require.Error(t, err)
			require.Nil(t, reader)
			require.Empty(t, native.calls)
		})
	}
	// An exact-mode/config claim for a different actual handle is not evidence.
	native := &recordingAdapter{}
	raw := dal.NewDB(native)
	fixture := newIdentityFixture(dal.NewDB(&recordingAdapter{}))
	db := identityDatabase(t, testAuthority(t, fixedResolver(qualifiedResolution())), raw, fixture)
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.Error(t, err)
	require.Nil(t, reader)
	require.Empty(t, native.calls)
	// Native aliases are refused by this fixture; spelling pg_temp cannot prove
	// which physical temporary schema a pooled execution would select.
	r := qualifiedResolution()
	r.Grants[0].Scope.Objects[0].Namespace = "pg_temp"
	db = identityDatabase(t, testAuthority(t, fixedResolver(r)), raw, newIdentityFixture(raw))
	reader, err = db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("pg_temp", "Customer", ""))
	require.Error(t, err)
	require.Nil(t, reader)
	require.Empty(t, native.calls)
}

func TestTableIdentityChangesDiscardOpenedAndStagedPages(t *testing.T) {
	for _, phase := range []string{"open", "next", "pre-page"} {
		t.Run(phase, func(t *testing.T) {
			nativeReader := &testReader{}
			native := &recordingAdapter{reader: nativeReader}
			raw := dal.NewDB(native)
			facts := newIdentityFixture(raw)
			change := func() {
				facts.tables[access.TableName{Schema: "sales", Name: "Customer"}] = access.TableName{Schema: "sales", Name: "Other"}
			}
			if phase == "open" {
				native.hook = func(context.Context) { change() }
			}
			if phase == "next" {
				nativeReader.next = func() (record.Record, error) {
					change()
					return record.NewRecordWithData(record.NewKeyWithID("Customer", "private"), map[string]any{"secret": true}), nil
				}
			}
			db := identityDatabase(t, testAuthority(t, fixedResolver(qualifiedResolution())), raw, facts)
			reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
			if phase == "open" {
				require.Error(t, err)
				require.Nil(t, reader)
			} else {
				require.NoError(t, err)
				if phase == "pre-page" {
					change()
				}
				rec, err := reader.Next()
				require.Error(t, err)
				require.Nil(t, rec)
				require.NoError(t, reader.Close())
			}
			require.Equal(t, 1, nativeReader.closeCalls)
		})
	}
}

func TestTableIdentityHeldPageRequiresSameCurrentMatchingGrant(t *testing.T) {
	for _, mode := range []string{"expiry", "revoke", "downscope", "replacement"} {
		t.Run(mode, func(t *testing.T) {
			r := qualifiedResolution()
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return r, nil })
			nativeReader := &testReader{}
			raw := dal.NewDB(&recordingAdapter{reader: nativeReader})
			facts := newIdentityFixture(raw)
			reader, err := identityDatabase(t, a, raw, facts).ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
			require.NoError(t, err)
			switch mode {
			case "expiry":
				r.Grants[0].ExpiresAt = r.ServerTime
			case "revoke":
				r.Grants[0].State = Revoked
			case "downscope":
				r.Grants[0].Scope.Objects[0].Name = "Other"
			case "replacement":
				r.Grants[0].ID = "replacement"
			}
			rec, err := reader.Next()
			require.Error(t, err)
			require.Nil(t, rec)
			require.Zero(t, nativeReader.nextCalls)
			require.NoError(t, reader.Close())
			require.EqualValues(t, 2, facts.calls.Load(), "failed permission candidate does not call facts")
		})
	}
}

func TestTableIdentityStallDoesNotBlockCloseCancelOrExpiry(t *testing.T) {
	for _, mode := range []string{"close", "cancel", "expiry", "fresh-expiry"} {
		t.Run(mode, func(t *testing.T) {
			r := qualifiedResolution()
			if mode == "expiry" {
				r.Grants[0].ExpiresAt = r.ServerTime.Add(150 * time.Millisecond)
			}
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return r, nil })
			nativeReader := &testReader{closed: make(chan struct{})}
			native := &recordingAdapter{reader: nativeReader}
			raw := dal.NewDB(native)
			facts := newIdentityFixture(raw)
			entered, unblock := make(chan context.Context, 1), make(chan struct{})
			var armed atomic.Bool
			verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
				if armed.Load() {
					entered <- ctx
					<-unblock
				}
				return facts.VerifyTableIdentity(ctx, actual, request)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			reader, err := identityDatabase(t, a, raw, verifier).ExecuteQueryToRecordsReader(ctx, operationQuery("sales", "Customer", ""))
			require.NoError(t, err)
			if mode == "fresh-expiry" {
				r.ValidUntil = r.ServerTime.Add(150 * time.Millisecond)
			}
			armed.Store(true)
			done := make(chan error, 1)
			go func() {
				rec, err := reader.Next()
				if rec != nil {
					done <- errors.New("private page escaped")
					return
				}
				done <- err
			}()
			var namingCtx context.Context
			select {
			case namingCtx = <-entered:
			case <-time.After(2 * time.Second):
				t.Fatal("fact port did not start")
			}
			deadline, ok := namingCtx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(min(r.Grants[0].ExpiresAt.Sub(r.ServerTime), r.ValidUntil.Sub(r.ServerTime))), deadline, time.Second)
			switch mode {
			case "close":
				closed := make(chan error, 1)
				go func() { closed <- reader.Close() }()
				select {
				case err := <-closed:
					require.NoError(t, err)
				case <-time.After(time.Second):
					t.Fatal("Close blocked on fact I/O")
				}
			case "cancel":
				cancel()
			case "expiry", "fresh-expiry":
				select {
				case <-namingCtx.Done():
				case <-time.After(2 * time.Second):
					t.Fatal("selected grant ceiling missing")
				}
			}
			select {
			case <-nativeReader.closed:
			case <-time.After(time.Second):
				t.Fatal("native reader not closed while facts blocked")
			}
			close(unblock)
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("Next stayed blocked after fact return")
			}
			require.Zero(t, nativeReader.nextCalls)
			require.NoError(t, reader.Close())
		})
	}
}

func TestTableIdentityInitialLateFactsNeverDispatch(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "local-clock"} {
		t.Run(mode, func(t *testing.T) {
			r := qualifiedResolution()
			native := &recordingAdapter{}
			raw := dal.NewDB(native)
			facts := newIdentityFixture(raw)
			a := testAuthority(t, fixedResolver(r))
			var ticks atomic.Int64
			ticks.Store(time.Now().UnixNano())
			a.now = func() time.Time { return time.Unix(0, ticks.Load()) }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			verifier := tableIdentityFunc(func(factCtx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
				deadline, ok := factCtx.Deadline()
				require.True(t, ok)
				require.WithinDuration(t, time.Now().Add(20*time.Second), deadline, time.Second)
				switch mode {
				case "cancel":
					cancel()
				case "deadline":
					return TableIdentityFacts{}, context.DeadlineExceeded
				default:
					ticks.Add(int64(time.Minute))
				}
				return facts.VerifyTableIdentity(factCtx, actual, request)
			})
			reader, err := identityDatabase(t, a, raw, verifier).ExecuteQueryToRecordsReader(ctx, operationQuery("sales", "Customer", ""))
			require.Error(t, err)
			require.Nil(t, reader)
			require.Empty(t, native.calls)
		})
	}
}

func TestTableIdentityBindingsArePerDatabaseAndOperation(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		raw := dal.NewDB(&recordingAdapter{})
		facts := newIdentityFixture(raw)
		db := identityDatabase(t, a, raw, facts)
		wg.Add(1)
		go func() {
			defer wg.Done()
			reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
			require.NoError(t, err)
			require.NoError(t, reader.Close())
			require.EqualValues(t, 2, facts.calls.Load())
		}()
	}
	wg.Wait()
}

func TestObjectGrantPreservesPointBaselineWithoutEnablingObjectPoints(t *testing.T) {
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		r := qualifiedResolution()
		r.Protection.ReadPosture = posture
		native := &recordingAdapter{}
		db := testDatabase(t, testAuthority(t, fixedResolver(r)), dal.NewDB(native))
		exists, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		if posture == ReadAll {
			require.NoError(t, err)
			require.True(t, exists)
			require.Equal(t, []string{"Exists"}, native.calls)
		} else {
			require.Error(t, err)
			require.False(t, exists)
			require.Empty(t, native.calls)
		}
	}
}

type valueIdentityFixture struct{ fixture *identityFixture }

func (f valueIdentityFixture) VerifyTableIdentity(ctx context.Context, db dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
	return f.fixture.VerifyTableIdentity(ctx, db, request)
}

type identityQueryAdapter struct {
	recordingAdapter
	query dal.Query
}

func (a *identityQueryAdapter) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	a.query = query
	return a.recordingAdapter.ExecuteQueryToRecordsReader(ctx, query)
}

func TestTableIdentityFreezesQueryAndFactInputs(t *testing.T) {
	native := &identityQueryAdapter{}
	raw := dal.NewDB(native)
	facts := newIdentityFixture(raw)
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	builder := dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "alias")).NewQuery().Limit(7).Offset(3).StartAfter("original")
	query := builder.SelectKeysOnly(reflect.String)
	verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
		got, err := (valueIdentityFixture{fixture: facts}).VerifyTableIdentity(ctx, actual, request)
		request.Connection.Space, request.Table.Name = "changed", "Other"
		builder.Limit(99).Offset(77).StartAfter("changed")
		return got, err
	})
	// Exercise non-nil value implementations as well as function/pointer ports.
	require.False(t, isNilValue(valueIdentityFixture{fixture: facts}))
	reader, err := identityDatabase(t, a, raw, verifier).ExecuteQueryToRecordsReader(context.Background(), query)
	require.NoError(t, err)
	frozen := native.query.(dal.StructuredQuery)
	require.Equal(t, 7, frozen.Limit())
	require.Equal(t, 3, frozen.Offset())
	require.Equal(t, dal.Cursor("original"), frozen.StartAfter())
	source := frozen.From().Base().(dal.CollectionRef)
	require.Equal(t, "alias", source.Alias())
	require.Equal(t, "Customer", source.Name())
	require.NoError(t, reader.Close())
	// Default ReadAll still cannot certify a new native naming/query shape.
	r := fixtureResolution()
	_, err = testDatabase(t, testAuthority(t, fixedResolver(r)), raw).ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.Error(t, err)
}

func TestTableIdentityRecheckCannotOutliveOriginalLocalCeiling(t *testing.T) {
	r := qualifiedResolution()
	a := testAuthority(t, fixedResolver(r))
	var ticks atomic.Int64
	ticks.Store(time.Now().UnixNano())
	a.now = func() time.Time { return time.Unix(0, ticks.Load()) }
	nativeReader := &testReader{}
	raw := dal.NewDB(&recordingAdapter{reader: nativeReader})
	facts := newIdentityFixture(raw)
	armed := false
	verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
		got, err := facts.VerifyTableIdentity(ctx, actual, request)
		if armed {
			ticks.Add(int64(time.Minute))
		}
		return got, err
	})
	reader, err := identityDatabase(t, a, raw, verifier).ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.NoError(t, err)
	armed = true
	rec, err := reader.Next()
	require.Error(t, err)
	require.Nil(t, rec)
	require.Zero(t, nativeReader.nextCalls)
	require.NoError(t, reader.Close())
}

func TestTableIdentityInitialStallKeepsSelectedCeiling(t *testing.T) {
	r := qualifiedResolution()
	r.Grants[0].ExpiresAt = r.ServerTime.Add(100 * time.Millisecond)
	native := &recordingAdapter{}
	raw := dal.NewDB(native)
	facts := newIdentityFixture(raw)
	entered, unblock := make(chan context.Context, 1), make(chan struct{})
	verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
		entered <- ctx
		<-unblock // Deliberately ignore cancellation to test the final guard.
		return facts.VerifyTableIdentity(ctx, actual, request)
	})
	db := identityDatabase(t, testAuthority(t, fixedResolver(r)), raw, verifier)
	done := make(chan error, 1)
	go func() {
		reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
		if reader != nil {
			done <- errors.New("late reader escaped")
			return
		}
		done <- err
	}()
	var factCtx context.Context
	select {
	case factCtx = <-entered:
	case <-time.After(time.Second):
		t.Fatal("fact port did not enter")
	}
	select {
	case <-factCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("unrelated B extended A's ceiling")
	}
	close(unblock)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("late fact return stayed blocked")
	}
	require.Empty(t, native.calls)
}

func TestTableIdentityMandatoryPolicyStillBlocksAdapterData(t *testing.T) {
	native := &recordingAdapter{}
	raw := dal.NewDB(native)
	facts := newIdentityFixture(raw)
	db := identityDatabase(t, testAuthority(t, fixedResolver(qualifiedResolution())), raw, facts)
	mandatory := access.MustPolicy("mandatory", access.TableScope(access.TableName{Schema: "sales", Name: "Customer"}, access.Deny(access.Read)))
	ctx := access.WithPolicy(context.Background(), mandatory)
	reader, err := db.ExecuteQueryToRecordsReader(ctx, operationQuery("sales", "Customer", ""))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Nil(t, reader)
	require.Empty(t, native.calls)
	require.EqualValues(t, 1, facts.calls.Load(), "trusted metadata lookup follows Core candidacy; restrictive policies still block data")
}

func TestTableIdentityFreshShorterCeilingClosesStalledNativePage(t *testing.T) {
	r := qualifiedResolution()
	var ttl atomic.Int64
	ttl.Store(int64(time.Minute))
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		fresh := r
		fresh.ServerTime = time.Now()
		fresh.ValidUntil = fresh.ServerTime.Add(time.Duration(ttl.Load()))
		return fresh, nil
	})
	entered, unblock := make(chan struct{}), make(chan struct{})
	nativeReader := &testReader{closed: make(chan struct{}), next: func() (record.Record, error) {
		close(entered)
		<-unblock
		return record.NewRecordWithData(record.NewKeyWithID("Customer", "private"), map[string]any{"secret": true}), nil
	}}
	raw := dal.NewDB(&recordingAdapter{reader: nativeReader})
	reader, err := identityDatabase(t, a, raw, newIdentityFixture(raw)).ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.NoError(t, err)
	// Cursor tightens once; Next tightens again, exercising timer reset while
	// the original selected grant and its unrelated B are both still active.
	ttl.Store(int64(time.Second))
	_, err = reader.Cursor()
	require.NoError(t, err)
	originalCeiling := reader.(*recordsReader).boundSnapshot().deadline
	ttl.Store(int64(time.Minute))
	_, err = reader.Cursor()
	require.NoError(t, err)
	require.Equal(t, originalCeiling, reader.(*recordsReader).boundSnapshot().deadline, "fresh authority never extends the retained minimum")
	ttl.Store(int64(100 * time.Millisecond))
	done := make(chan error, 1)
	go func() {
		rec, err := reader.Next()
		if rec != nil {
			done <- errors.New("late native page escaped")
			return
		}
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("native page did not enter")
	}
	select {
	case <-nativeReader.closed:
	case <-time.After(time.Second):
		t.Fatal("fresh shorter ceiling did not close native reader")
	}
	close(unblock)
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("page did not finish")
	}
	var closes sync.WaitGroup
	for range 4 {
		closes.Add(1)
		go func() { defer closes.Done(); require.NoError(t, reader.Close()) }()
	}
	closes.Wait()
	nativeReader.mu.Lock()
	require.Equal(t, 1, nativeReader.closeCalls, "timer/cancellation/concurrent Close share one native close")
	nativeReader.mu.Unlock()
}

func TestReaderCeilingHookCancellationPrecedesNaming(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _, _, err := a.prepareWithCeiling(ctx, nil, operationRequest(operationQuery("sales", "Customer", "")), func(*lease) { cancel() })
	require.ErrorIs(t, err, context.Canceled)
}

func TestTableIdentityPostOpenStallOwnsAndClosesUnpublishedReader(t *testing.T) {
	for _, mode := range []string{"shorter-expiry", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			r := qualifiedResolution()
			var resolutions atomic.Int64
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				fresh := r
				fresh.ServerTime = time.Now()
				if resolutions.Add(1) > 1 && mode == "shorter-expiry" {
					fresh.ValidUntil = fresh.ServerTime.Add(100 * time.Millisecond)
				}
				return fresh, nil
			})
			nativeReader := &testReader{closed: make(chan struct{})}
			raw := dal.NewDB(&recordingAdapter{reader: nativeReader})
			facts := newIdentityFixture(raw)
			entered, unblock := make(chan context.Context, 1), make(chan struct{})
			var lookups atomic.Int64
			verifier := tableIdentityFunc(func(ctx context.Context, actual dal.DB, request TableIdentityRequest) (TableIdentityFacts, error) {
				if lookups.Add(1) == 2 {
					entered <- ctx
					<-unblock
				}
				return facts.VerifyTableIdentity(ctx, actual, request)
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			db := identityDatabase(t, a, raw, verifier)
			done := make(chan error, 1)
			go func() {
				reader, err := db.ExecuteQueryToRecordsReader(ctx, operationQuery("sales", "Customer", ""))
				if reader != nil {
					done <- errors.New("unverified reader escaped")
					return
				}
				done <- err
			}()
			var factCtx context.Context
			select {
			case factCtx = <-entered:
			case <-time.After(time.Second):
				t.Fatal("post-open facts did not start")
			}
			if mode == "cancel" {
				cancel()
			}
			select {
			case <-factCtx.Done():
			case <-time.After(time.Second):
				t.Fatal("post-open context ceiling missing")
			}
			select {
			case <-nativeReader.closed:
			case <-time.After(time.Second):
				t.Fatal("unpublished reader stayed open during blocked facts")
			}
			close(unblock)
			select {
			case err := <-done:
				require.Error(t, err)
			case <-time.After(time.Second):
				t.Fatal("post-open call stayed blocked")
			}
			nativeReader.mu.Lock()
			require.Equal(t, 1, nativeReader.closeCalls)
			nativeReader.mu.Unlock()
		})
	}
}

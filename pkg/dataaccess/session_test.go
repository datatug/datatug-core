package dataaccess

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func TestConnectionReadGrantAndRevocationAreSharedCoreDecisions(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	for _, raw := range []dal.DB{dal.NewDB(&recordingAdapter{}), newMemoryRecorder(t)} {
		a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
		db := testDatabase(t, a, raw)
		exists, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.NoError(t, err)
		require.True(t, exists)
		resolution.Grants[0].State = Revoked
		exists, err = db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.ErrorIs(t, err, access.ErrAccessDenied)
		require.False(t, exists)
		resolution.Grants[0].State = Approved
	}
	resolution.Grants[0].ExpiresAt = resolution.ServerTime
	db := testDatabase(t, testAuthority(t, fixedResolver(resolution)), dal.NewDB(&recordingAdapter{}))
	_, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
}

func TestAgentUsesBaselineAndNeverInheritsHumanGrant(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Subject.Kind = Agent
	newBoundary := func() Database {
		a, err := NewAuthority(resolution.Subject, resolution.Connection, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
		require.NoError(t, err)
		return testDatabase(t, a, dal.NewDB(&recordingAdapter{}))
	}
	db := newBoundary()
	exists, err := db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.NoError(t, err)
	require.True(t, exists)
	resolution.Protection.AgentRead = AgentNone
	_, err = db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	resolution.Protection.AgentRead = AgentInherit
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	_, err = db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	resolution.Grants[0].Subject = resolution.Subject
	_, err = db.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
}

func TestCORE2UnknownRawAndComplexShapesNeverDispatch(t *testing.T) {
	adapter := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
	queries := []dal.Query{nil, dal.NewTextQuery("SELECT side_effect()", nil),
		dal.From(dal.NewRootCollectionRef("public.docs", "")).NewQuery().SelectKeysOnly(reflect.String),
		dal.From(dal.NewRootCollectionRef("docs", "alias")).NewQuery().SelectKeysOnly(reflect.String),
		dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().WhereField("secret", dal.Equal, "value").SelectKeysOnly(reflect.String),
		dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().SelectIntoRecordset(),
		dal.From(dal.NewRootCollectionRef("docs", "")).Join(dal.NewJoinedSource(dal.NewRootCollectionRef("other", ""), dal.JoinInner)).NewQuery().SelectKeysOnly(reflect.String),
	}
	for _, query := range queries {
		reader, err := db.ExecuteQueryToRecordsReader(context.Background(), query)
		require.ErrorIs(t, err, access.ErrAccessDenied)
		require.Nil(t, reader)
	}
	reader, err := db.ExecuteQueryToRecordsetReader(context.Background(), keyQuery())
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Nil(t, reader)
	require.Empty(t, adapter.calls)
	// Cursor and paging shape are frozen into an ordinary DALgo query.
	for _, startFrom := range []bool{false, true} {
		builder := dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().Limit(5).Offset(2)
		if startFrom {
			builder.StartFrom("from")
		} else {
			builder.StartAfter("after")
		}
		frozen, ok := freezeKeysQuery(builder.SelectKeysOnly(reflect.String))
		require.True(t, ok)
		require.Equal(t, 5, frozen.Limit())
		require.Equal(t, 2, frozen.Offset())
		if startFrom {
			require.Equal(t, dal.Cursor("from"), frozen.(dal.StructuredQuery).StartFrom())
		} else {
			require.Equal(t, dal.Cursor("after"), frozen.(dal.StructuredQuery).StartAfter())
		}
	}
}

func TestPointReadDoesNotReleaseValuesAfterExpiryOrRevoke(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(map[bool]string{false: "expiry", true: "revocation"}[revoke], func(t *testing.T) {
			resolution := fixtureResolution()
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
			now := time.Now()
			a.now = func() time.Time { return now }
			adapter := &recordingAdapter{hook: func(context.Context) {
				if revoke {
					resolution.Protection.ReadPosture = ReadNone
				} else {
					now = now.Add(2 * time.Minute)
				}
			}}
			db := testDatabase(t, a, dal.NewDB(adapter))
			data := map[string]any{"value": "private"}
			rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), data)
			require.ErrorIs(t, db.Get(context.Background(), rec), access.ErrAccessDenied)
			require.Empty(t, data)
			require.ErrorIs(t, rec.Error(), access.ErrAccessDenied)
		})
	}
	// A caller-owned struct target is also cleared on a post-I/O denial.
	resolution := fixtureResolution()
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
	adapter := &recordingAdapter{hook: func(context.Context) { resolution.Protection.ReadPosture = ReadNone }}
	db := testDatabase(t, a, dal.NewDB(adapter))
	data := struct{ Value string }{Value: "private"}
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), &data)
	require.Error(t, db.GetMulti(context.Background(), []record.Record{rec}))
	require.Empty(t, data.Value)
}

func TestReadErrorsAndMissingAuthorityNeverBecomeEmptySuccess(t *testing.T) {
	resolution := fixtureResolution()
	adapter := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil }), dal.NewDB(adapter))
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{})
	require.NoError(t, db.Get(context.Background(), rec))
	resolution.Protection.ReadPosture = ReadNone
	require.ErrorIs(t, db.Get(context.Background(), rec), access.ErrAccessDenied)
	require.ErrorIs(t, rec.Error(), access.ErrAccessDenied)
	_, err := db.ExecuteQueryToRecordsReader(context.Background(), keyQuery())
	require.ErrorIs(t, err, access.ErrAccessDenied)
	resolution.Protection.ReadPosture = ReadAll
	adapter.err = errors.New("adapter failed")
	_, err = db.ExecuteQueryToRecordsReader(context.Background(), keyQuery())
	require.EqualError(t, err, "adapter failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := len(adapter.calls)
	_, err = db.Exists(ctx, rec.Key())
	require.ErrorIs(t, err, context.Canceled)
	require.Len(t, adapter.calls, before)
}

func TestCORE5PinnedPolicyCannotExtendTransactionOrRetainedHandle(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(80 * time.Millisecond)
	var grantWasExtended bool
	resolver := func(context.Context, ResolveRequest) (Resolution, error) {
		copy := resolution
		copy.ServerTime = time.Now()
		if grantWasExtended {
			copy.Grants = []Grant{fixtureGrant(copy.ServerTime)}
		}
		return copy, nil
	}
	adapter := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, resolver), dal.NewDB(adapter))
	var retained dal.ReadTransaction
	err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
		retained = tx
		require.Same(t, tx, dal.GetTransaction(ctx))
		require.Equal(t, "checkpoint", tx.Options().Message())
		_, writable := tx.(dal.WriteSession)
		require.False(t, writable)
		exists, err := tx.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.NoError(t, err)
		require.True(t, exists)
		grantWasExtended = true
		// DALgo pinned its initial policy, but the wrapper's original server
		// deadline remains a real context timer even while the worker is idle.
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("transaction was not cancelled at its original deadline")
		}
		before := len(adapter.calls)
		_, err = tx.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.Error(t, err)
		require.Len(t, adapter.calls, before)
		return nil
	}, dal.TxWithMessage("checkpoint"))
	require.ErrorIs(t, err, access.ErrAccessDenied)
	before := len(adapter.calls)
	_, err = retained.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.Error(t, err)
	require.Len(t, adapter.calls, before)
}

func TestRetainedTransactionAndReaderEndWithCallback(t *testing.T) {
	rawReader := &testReader{closed: make(chan struct{})}
	adapter := &recordingAdapter{reader: rawReader}
	db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
	var retained dal.ReadTransaction
	var reader dal.RecordsReader
	err := db.RunReadonlyTransaction(context.Background(), func(_ context.Context, tx dal.ReadTransaction) error {
		retained = tx
		var err error
		reader, err = tx.ExecuteQueryToRecordsReader(context.Background(), keyQuery())
		return err
	})
	require.NoError(t, err)
	select {
	case <-rawReader.closed:
	case <-time.After(time.Second):
		t.Fatal("retained reader was not closed with transaction")
	}
	_, err = retained.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, context.Canceled)
	value, err := reader.Cursor()
	require.Error(t, err)
	require.Empty(t, value)
	// Worker errors, refusal before starting a transaction, and provider errors
	// are propagated as errors, without invoking a callback as success.
	cause := errors.New("worker error")
	require.ErrorIs(t, db.RunReadonlyTransaction(context.Background(), func(context.Context, dal.ReadTransaction) error { return cause }), cause)
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	denied := testDatabase(t, testAuthority(t, fixedResolver(resolution)), dal.NewDB(adapter))
	require.ErrorIs(t, denied.RunReadonlyTransaction(context.Background(), func(context.Context, dal.ReadTransaction) error { t.Fatal("denied worker ran"); return nil }), access.ErrAccessDenied)
}

func TestServerTimeBudgetIsConservativeAndCannotBeRenewed(t *testing.T) {
	resolution := fixtureResolution()
	resolution.ServerTime = resolution.ServerTime.Add(24 * time.Hour)
	resolution.ValidUntil = resolution.ServerTime.Add(5 * time.Minute)
	resolution.Protection.ReadPosture = ReadNone
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(20 * time.Second)
	local := time.Now()
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		local = local.Add(3 * time.Second)
		return resolution, nil
	})
	a.now = func() time.Time { return local }
	started := local
	ctx, authorization, cancel, err := a.prepare(context.Background(), nil)
	require.NoError(t, err)
	defer cancel()
	require.Equal(t, started.Add(20*time.Second), authorization.deadline)
	// A refresh after three seconds retains the earlier original deadline.
	_, refreshed, stop, err := a.prepare(ctx, authorization)
	require.NoError(t, err)
	defer stop()
	require.Equal(t, authorization.deadline, refreshed.deadline)
	local = local.Add(time.Minute)
	_, _, _, err = a.prepare(ctx, authorization)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	// Resolution latency alone can exhaust the whole authority interval.
	a = testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		local = local.Add(2 * time.Minute)
		return resolution, nil
	})
	a.now = func() time.Time { return local }
	_, _, _, err = a.prepare(context.Background(), nil)
	require.ErrorIs(t, err, access.ErrAccessDenied)
}

func TestAddressFallbackAndIdentityHintAreDistinct(t *testing.T) {
	a := Fingerprint{Engine: "engine", Database: "db", Address: "one"}
	b := a
	require.True(t, sameFingerprint(a, b))
	b.Address = "two"
	require.False(t, sameFingerprint(a, b))
	a.EngineIdentity = "lineage"
	b.EngineIdentity = "lineage"
	require.True(t, sameFingerprint(a, b))
}

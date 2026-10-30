package dataaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func openTestReader(t *testing.T, authority *Authority, raw *testReader) dal.RecordsReader {
	t.Helper()
	db := testDatabase(t, authority, dal.NewDB(&recordingAdapter{reader: raw}))
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), keyQuery())
	require.NoError(t, err)
	return reader
}

func TestCORE5ActualDeadlineClosesIdleReaderAndRefusesCursor(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(60 * time.Millisecond)
	raw := &testReader{closed: make(chan struct{})}
	reader := openTestReader(t, testAuthority(t, fixedResolver(resolution)), raw)
	select {
	case <-raw.closed:
	case <-time.After(time.Second):
		t.Fatal("idle reader did not close at the real authorization deadline")
	}
	value, err := reader.Cursor()
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Empty(t, value)
	rec, err := reader.Next()
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Nil(t, rec)
	require.NoError(t, reader.Close())
	raw.mu.Lock()
	defer raw.mu.Unlock()
	require.Zero(t, raw.cursorCalls)
	require.Zero(t, raw.nextCalls)
	require.Equal(t, 1, raw.closeCalls)
}

func TestReaderReleasesAllowedRecordsAndCursorsAndClosesOnce(t *testing.T) {
	raw := &testReader{}
	reader := openTestReader(t, testAuthority(t, fixedResolver(fixtureResolution())), raw)
	rec, err := reader.Next()
	require.NoError(t, err)
	require.Equal(t, "one", rec.Key().ID)
	cursor, err := reader.Cursor()
	require.NoError(t, err)
	require.Equal(t, "private-cursor", cursor)
	require.NoError(t, reader.Close())
	require.NoError(t, reader.Close())
	rec, err = reader.Next()
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Nil(t, rec)
	require.Equal(t, 1, raw.closeCalls)
}

func TestReaderRefusesPostIOExpiryAndRevocation(t *testing.T) {
	for _, operation := range []string{"Next", "Cursor"} {
		for _, revoke := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "expiry", true: "revoke"}[revoke], func(t *testing.T) {
				resolution := fixtureResolution()
				authority := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
				now := time.Now()
				authority.now = func() time.Time { return now }
				invalidate := func() {
					if revoke {
						resolution.Protection.ReadPosture = ReadNone
					} else {
						now = now.Add(2 * time.Minute)
					}
				}
				raw := &testReader{
					next: func() (record.Record, error) {
						invalidate()
						return record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"secret": "value"}), nil
					},
					cursor: func() (string, error) { invalidate(); return "secret-cursor", nil },
				}
				reader := openTestReader(t, authority, raw)
				if operation == "Next" {
					rec, err := reader.Next()
					require.ErrorIs(t, err, access.ErrAccessDenied)
					require.Nil(t, rec)
				} else {
					cursor, err := reader.Cursor()
					require.ErrorIs(t, err, access.ErrAccessDenied)
					require.Empty(t, cursor)
				}
				require.Equal(t, 1, raw.closeCalls)
				require.NoError(t, reader.Close())
			})
		}
	}
}

func TestReaderRevalidationFailureStopsBeforePageAndDropsUpstreamSecrets(t *testing.T) {
	resolution := fixtureResolution()
	unavailable := false
	authority := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		if unavailable {
			return Resolution{}, errors.New("upstream-secret-token")
		}
		return resolution, nil
	})
	raw := &testReader{}
	reader := openTestReader(t, authority, raw)
	unavailable = true
	rec, err := reader.Next()
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.NotContains(t, err.Error(), "secret")
	require.Nil(t, rec)
	require.Zero(t, raw.nextCalls)
	require.Equal(t, 1, raw.closeCalls)
	require.NoError(t, reader.Close())
}

func TestBlockedReaderCannotReleaseAfterRealDeadline(t *testing.T) {
	resolution := fixtureResolution()
	resolution.ValidUntil = resolution.ServerTime.Add(60 * time.Millisecond)
	var queryContext context.Context
	raw := &testReader{next: func() (record.Record, error) {
		<-queryContext.Done()
		// A late provider result is discarded even if it returns data after the
		// context cancellation instead of returning a deadline error itself.
		return record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"secret": "late"}), nil
	}}
	adapter := &recordingAdapter{reader: raw, hook: func(ctx context.Context) { queryContext = ctx }}
	db := testDatabase(t, testAuthority(t, fixedResolver(resolution)), dal.NewDB(adapter))
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), keyQuery())
	require.NoError(t, err)
	rec, err := reader.Next()
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.Nil(t, rec)
	require.NoError(t, reader.Close())
}

func TestParentCancellationClosesReaderAndAllReleasesFail(t *testing.T) {
	raw := &testReader{closed: make(chan struct{})}
	db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(&recordingAdapter{reader: raw}))
	ctx, cancel := context.WithCancel(context.Background())
	reader, err := db.ExecuteQueryToRecordsReader(ctx, keyQuery())
	require.NoError(t, err)
	cancel()
	select {
	case <-raw.closed:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation did not close reader")
	}
	cursor, err := reader.Cursor()
	require.ErrorIs(t, err, context.Canceled)
	require.Empty(t, cursor)
	rec, err := reader.Next()
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, rec)
	require.NoError(t, reader.Close())
}

func TestReaderPropagatesEOFAndCloseFailure(t *testing.T) {
	raw := &testReader{next: func() (record.Record, error) { return nil, dal.ErrNoMoreRecords }, closeErr: errors.New("close failed")}
	reader := openTestReader(t, testAuthority(t, fixedResolver(fixtureResolution())), raw)
	rec, err := reader.Next()
	require.ErrorIs(t, err, dal.ErrNoMoreRecords)
	require.Nil(t, rec)
	require.EqualError(t, reader.Close(), "close failed")
}

func TestTransactionOperationAuthorityFailureDoesNotDispatch(t *testing.T) {
	resolution := fixtureResolution()
	adapter := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil }), dal.NewDB(adapter))
	err := db.RunReadonlyTransaction(context.Background(), func(_ context.Context, tx dal.ReadTransaction) error {
		resolution.Protection.ReadPosture = ReadNone
		before := len(adapter.calls)
		_, err := tx.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
		require.ErrorIs(t, err, access.ErrAccessDenied)
		require.Len(t, adapter.calls, before)
		return err
	})
	require.ErrorIs(t, err, access.ErrAccessDenied)
}

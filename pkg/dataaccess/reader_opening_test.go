package dataaccess

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

// openingReplyDB preserves the real DALgo DB surfaces while supplying the
// provider's precise malformed/partial return pair at the opened-handle seam.
type openingReplyDB struct {
	dal.DB
	reply dal.RecordsReader
	err   error
	hook  func()
	ctx   context.Context
}

func (db *openingReplyDB) ExecuteQueryToRecordsReader(ctx context.Context, _ dal.Query) (dal.RecordsReader, error) {
	db.ctx = ctx
	if db.hook != nil {
		db.hook()
	}
	return db.reply, db.err
}

func TestReaderOpeningErrorClosesPartialHandleAndPreservesPrimaryError(t *testing.T) {
	for _, mode := range []string{"partial", "close-error", "expired-partial", "nil", "typed-nil"} {
		t.Run(mode, func(t *testing.T) {
			openErr, closeErr := errors.New("provider open failed"), errors.New("provider close failed")
			native := &testReader{}
			raw := &openingReplyDB{DB: dal.NewDB(&recordingAdapter{}), reply: native, err: openErr}
			a := testAuthority(t, fixedResolver(qualifiedResolution()))
			now := time.Now()
			a.now = func() time.Time { return now }
			switch mode {
			case "close-error":
				native.closeErr = closeErr
			case "expired-partial":
				raw.hook = func() { now = now.Add(time.Minute) }
			case "nil":
				raw.reply = nil
			case "typed-nil":
				var absent *testReader
				raw.reply = absent
			}
			facts := newIdentityFixture(raw)
			db := identityDatabase(t, a, raw, facts)
			var reader dal.RecordsReader
			var err error
			require.NotPanics(t, func() {
				reader, err = db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
			})
			require.Nil(t, reader)
			require.ErrorIs(t, err, openErr)
			require.NotErrorIs(t, err, closeErr)
			require.ErrorIs(t, raw.ctx.Err(), context.Canceled, "the prepared operation context is canceled on every failed open")
			if mode == "nil" || mode == "typed-nil" {
				require.Zero(t, native.closeCalls)
			} else {
				require.Equal(t, 1, native.closeCalls)
			}
			require.EqualValues(t, 1, facts.calls.Load(), "failed open never enters post-open fact/watch phase")
		})
	}
}

func TestReaderOpeningNilSuccessRefusesBeforeWatcherPublication(t *testing.T) {
	for _, typed := range []bool{false, true} {
		for _, qualified := range []bool{false, true} {
			t.Run(map[bool]string{false: "raw-nil", true: "typed-nil"}[typed]+map[bool]string{false: "-default", true: "-qualified"}[qualified], func(t *testing.T) {
				raw := &openingReplyDB{DB: dal.NewDB(&recordingAdapter{})}
				if typed {
					var absent *testReader
					raw.reply = absent
				}
				a := testAuthority(t, fixedResolver(fixtureResolution()))
				facts := newIdentityFixture(raw)
				var db Database
				query := keyQuery()
				if qualified {
					db = identityDatabase(t, a, raw, facts)
					query = operationQuery("sales", "Customer", "")
				} else {
					db = testDatabase(t, a, raw)
				}
				var reader dal.RecordsReader
				var err error
				require.NotPanics(t, func() { reader, err = db.ExecuteQueryToRecordsReader(context.Background(), query) })
				require.Nil(t, reader)
				require.ErrorIs(t, err, access.ErrAccessDenied)
				require.ErrorIs(t, raw.ctx.Err(), context.Canceled)
				if qualified {
					require.EqualValues(t, 1, facts.calls.Load())
				} else {
					require.Zero(t, facts.calls.Load())
				}
			})
		}
	}
}

func TestReaderOpenedDuringExpiryClosesOnceDespiteCleanupFailure(t *testing.T) {
	closeErr := errors.New("native close failure")
	native := &testReader{closeErr: closeErr}
	raw := &openingReplyDB{DB: dal.NewDB(&recordingAdapter{}), reply: native}
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	now := time.Now()
	a.now = func() time.Time { return now }
	raw.hook = func() { now = now.Add(time.Minute) }
	db := identityDatabase(t, a, raw, newIdentityFixture(raw))
	reader, err := db.ExecuteQueryToRecordsReader(context.Background(), operationQuery("sales", "Customer", ""))
	require.Nil(t, reader)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	require.NotErrorIs(t, err, closeErr)
	require.ErrorIs(t, raw.ctx.Err(), context.Canceled)
	native.mu.Lock()
	require.Equal(t, 1, native.closeCalls)
	native.mu.Unlock()
}

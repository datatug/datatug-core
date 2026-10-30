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

// REVIEW r1 A: a provider may use Close, rather than context cancellation, to
// unblock its context-free Next/Cursor methods. A deadline must still close it.
func TestDeadlineClosesProviderWhileNextOrCursorIsBlocked(t *testing.T) {
	for _, operation := range []string{"Next", "Cursor"} {
		t.Run(operation, func(t *testing.T) {
			resolution := fixtureResolution()
			resolution.ValidUntil = resolution.ServerTime.Add(80 * time.Millisecond)
			closed := make(chan struct{})
			raw := &testReader{closed: closed,
				next: func() (record.Record, error) {
					<-closed
					return record.NewRecordWithData(record.NewKeyWithID("docs", "one"), map[string]any{"private": true}), nil
				},
				cursor: func() (string, error) { <-closed; return "private-token", nil },
			}
			reader := openTestReader(t, testAuthority(t, fixedResolver(resolution)), raw)
			done := make(chan error, 1)
			go func() {
				if operation == "Next" {
					rec, err := reader.Next()
					if rec != nil {
						done <- errors.New("late record escaped")
						return
					}
					done <- err
				} else {
					cursor, err := reader.Cursor()
					if cursor != "" {
						done <- errors.New("late cursor escaped")
						return
					}
					done <- err
				}
			}()
			select {
			case err := <-done:
				require.ErrorIs(t, err, access.ErrAccessDenied)
			case <-time.After(time.Second):
				t.Fatal("provider Close was blocked by its active operation")
			}
			require.NoError(t, reader.Close())
		})
	}
}

func TestManualCloseAndDeadlineDoNotWaitForResolverIO(t *testing.T) {
	for _, manual := range []bool{false, true} {
		t.Run(map[bool]string{false: "deadline", true: "manual"}[manual], func(t *testing.T) {
			resolution := fixtureResolution()
			resolution.ValidUntil = resolution.ServerTime.Add(80 * time.Millisecond)
			block := false
			entered, unblock := make(chan struct{}), make(chan struct{})
			authority := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				if block {
					close(entered)
					<-unblock
				}
				return resolution, nil
			})
			closed := make(chan struct{})
			reader := openTestReader(t, authority, &testReader{closed: closed})
			block = true
			done := make(chan error, 1)
			go func() { _, err := reader.Next(); done <- err }()
			<-entered
			if manual {
				require.NoError(t, reader.Close())
			}
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("close waited for resolver I/O")
			}
			close(unblock)
			require.Error(t, <-done)
			require.NoError(t, reader.Close())
		})
	}
}

// REVIEW r1 B: the adapter writes only to staging containers. Neither partial
// error nor post-I/O denial can expose freshly loaded data in caller aliases.
func TestStagedPointReadsDropPartialErrorExpiryAndRevocation(t *testing.T) {
	type customTarget struct{ Secret string }
	for _, failure := range []string{"partial", "expiry", "revocation"} {
		for _, shape := range []string{"map", "pointer", "slice"} {
			t.Run(failure+shape, func(t *testing.T) {
				resolution := fixtureResolution()
				authority := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
				now := time.Now()
				authority.now = func() time.Time { return now }
				mapTarget := map[string]any{}
				pointerTarget := &customTarget{}
				sliceTarget := []string{""}
				var original any = mapTarget
				if shape == "pointer" {
					original = pointerTarget
				} else if shape == "slice" {
					original = sliceTarget
				}
				adapter := &recordingAdapter{load: func(records []record.Record) {
					switch shape {
					case "map":
						records[0].Data().(map[string]any)["secret"] = "new-private-value"
					case "pointer":
						records[0].Data().(*customTarget).Secret = "new-private-value"
					case "slice":
						records[0].Data().([]string)[0] = "new-private-value"
					}
					// The original aliases stay empty even before the provider
					// returns, while a caller could still hold those aliases.
					require.Empty(t, mapTarget)
					require.Empty(t, pointerTarget.Secret)
					require.Empty(t, sliceTarget[0])
				}}
				if failure == "partial" {
					adapter.err = errors.New("provider partial error")
				}
				adapter.hook = func(context.Context) {
					if failure == "expiry" {
						now = now.Add(2 * time.Minute)
					}
					if failure == "revocation" {
						resolution.Protection.ReadPosture = ReadNone
					}
				}
				db := testDatabase(t, authority, dal.NewDB(adapter))
				rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), original)
				require.Error(t, db.Get(context.Background(), rec))
				require.Empty(t, mapTarget)
				require.Empty(t, pointerTarget.Secret)
				require.Empty(t, sliceTarget[0])
				require.Error(t, rec.Error())
			})
		}
	}
}

func TestStagedPointReadPublishesOnlySuccessfulAuthorizedResults(t *testing.T) {
	type customTarget struct{ Value string }
	for _, shape := range []string{"map", "pointer", "slice"} {
		t.Run(shape, func(t *testing.T) {
			mapTarget := map[string]any{"old": "stale"}
			pointerTarget := &customTarget{Value: "old"}
			sliceTarget := []string{"old"}
			var original any = mapTarget
			if shape == "pointer" {
				original = pointerTarget
			} else if shape == "slice" {
				original = sliceTarget
			}
			adapter := &recordingAdapter{load: func(records []record.Record) {
				switch shape {
				case "map":
					records[0].Data().(map[string]any)["value"] = "new"
				case "pointer":
					records[0].Data().(*customTarget).Value = "new"
				case "slice":
					records[0].Data().([]string)[0] = "new"
				}
			}}
			db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
			rec := record.NewRecordWithData(record.NewKeyWithID("docs", "one"), original)
			require.NoError(t, db.Get(context.Background(), rec))
			require.True(t, rec.Exists())
			if shape == "map" {
				require.Equal(t, map[string]any{"value": "new"}, mapTarget)
			}
			if shape == "pointer" {
				require.Equal(t, "new", pointerTarget.Value)
			}
			if shape == "slice" {
				require.Equal(t, []string{"new"}, sliceTarget)
			}
		})
	}
	adapter := &recordingAdapter{missing: true}
	db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
	rec := record.NewRecordWithData(record.NewKeyWithID("docs", "missing"), map[string]any{})
	require.NoError(t, db.Get(context.Background(), rec))
	require.False(t, rec.Exists())
}

func TestPointReadRefusesInaccessibleAndUnsupportedTargetsBeforeDispatch(t *testing.T) {
	key := record.NewKeyWithID("docs", "one")
	var absentMap map[string]any
	var absentPointer *struct{ Value string }
	var absentSlice []string
	for _, rec := range []record.Record{nil, record.NewRecord(key), record.NewRecordWithData(key, 3),
		record.NewRecordWithData(key, absentMap), record.NewRecordWithData(key, absentPointer), record.NewRecordWithData(key, absentSlice)} {
		adapter := &recordingAdapter{}
		db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
		require.ErrorIs(t, db.Get(context.Background(), rec), access.ErrAccessDenied)
		require.Empty(t, adapter.calls)
	}
}

// REVIEW r1 C: foreign/typed-nil AST objects must be refused without calling
// methods that can panic or change meaning between inspection and dispatch.
type foreignQuery struct{ dal.StructuredQuery }
type foreignFrom struct{ dal.FromSource }

func TestUnknownAndTypedNilQueryImplementationsAreRefused(t *testing.T) {
	var nilQuery *foreignQuery
	var nilFrom *foreignFrom
	var queries = []dal.Query{nilQuery, &foreignQuery{},
		dal.NewQueryBuilder(nilFrom).SelectKeysOnly(reflect.String),
		dal.NewQueryBuilder(reflect.Zero(keysFromType).Interface().(dal.FromSource)).SelectKeysOnly(reflect.String),
	}
	adapter := &recordingAdapter{}
	db := testDatabase(t, testAuthority(t, fixedResolver(fixtureResolution())), dal.NewDB(adapter))
	for _, query := range queries {
		reader, err := db.ExecuteQueryToRecordsReader(context.Background(), query)
		require.ErrorIs(t, err, access.ErrAccessDenied)
		require.Nil(t, reader)
	}
	require.Empty(t, adapter.calls)
}

// REVIEW r1 D: grants union equivalent read authority; they never shorten an
// existing standing permission or another valid connection read grant.
func TestConnectionGrantUnionNeverNarrowsReadLifetime(t *testing.T) {
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		for _, reverse := range []bool{false, true} {
			resolution := fixtureResolution()
			resolution.ValidUntil = resolution.ServerTime.Add(2 * time.Minute)
			resolution.Protection.ReadPosture = posture
			long, short := fixtureGrant(resolution.ServerTime), fixtureGrant(resolution.ServerTime)
			long.ExpiresAt = resolution.ServerTime.Add(40 * time.Second)
			short.ID = "grant-2"
			short.ExpiresAt = resolution.ServerTime.Add(time.Second)
			resolution.Grants = []Grant{long, short}
			if reverse {
				resolution.Grants = []Grant{short, long}
			}
			authority := testAuthority(t, fixedResolver(resolution))
			now := time.Now()
			authority.now = func() time.Time { return now }
			authorization, err := authority.authorize(context.Background())
			require.NoError(t, err)
			want := time.Minute
			if posture == ReadNone {
				want = 40 * time.Second
			}
			require.Equal(t, now.Add(want), authorization.deadline)
		}
	}
}

// REVIEW r1 E: malformed contracts and understood-but-unavailable scope
// features have separate stable error codes, including null/casefolded fields.
func TestUnsupportedScopePresenceHasStableCode(t *testing.T) {
	for _, field := range []string{"schemas", "SCHEMAS", "notBefore", "NOTBEFORE"} {
		_, err := DecodeAccessRequest([]byte(`{"scope":{"connection":"conn","requires":[],"` + field + `":null}}`))
		var denial *Denial
		require.ErrorAs(t, err, &denial)
		require.Equal(t, "UNSUPPORTED_SCOPE", denial.Code)
	}
	_, err := DecodeAccessRequest([]byte(`{"unknown":true}`))
	var denial *Denial
	require.ErrorAs(t, err, &denial)
	require.Equal(t, "INVALID_CONTRACT", denial.Code)
}

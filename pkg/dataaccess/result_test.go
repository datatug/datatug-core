package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func resultRequest() ResultRequest {
	return ResultRequest{Nonce: "fresh-result-nonce-123", RequestSHA256: hashBytes([]byte("Customers/one"))}
}
func captureFixture(t *testing.T, a *Authority, ctx context.Context) *Result {
	t.Helper()
	r, err := a.CaptureResult(ctx, resultRequest(), func(context.Context) ([]byte, error) { return []byte(` {"record":{"id":"one"}} `), nil })
	require.NoError(t, err)
	t.Cleanup(r.Close)
	return r
}
func envelopeFixture(t *testing.T, a *Authority) (ResultEnvelope, time.Time) {
	t.Helper()
	started := time.Now()
	r := captureFixture(t, a, context.Background())
	var buf bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &buf))
	envelope, err := DecodeResultEnvelope(buf.Bytes())
	require.NoError(t, err)
	return envelope, started
}

func TestResultRoundTripAndImmutableBytes(t *testing.T) {
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	input := []byte(`{"record":{"name":"Ada"}}`)
	r, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return input, nil })
	require.NoError(t, err)
	defer r.Close()
	clear(input)
	var payload bytes.Buffer
	require.NoError(t, r.WritePayload(context.Background(), &payload))
	require.JSONEq(t, `{"record":{"name":"Ada"}}`, payload.String())
	var wire bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &wire))
	envelope, err := DecodeResultEnvelope(wire.Bytes())
	require.NoError(t, err)
	imported, err := a.ImportResult(context.Background(), resultRequest(), time.Now(), envelope)
	require.NoError(t, err)
	defer imported.Close()
	clear(envelope.Payload)
	envelope.Lease.Subject.ID = "attacker"
	payload.Reset()
	require.NoError(t, imported.WritePayload(context.Background(), &payload))
	require.Contains(t, payload.String(), "Ada")
	imported.Close()
	require.Error(t, imported.WritePayload(context.Background(), &payload))
	var nilResult *Result
	nilResult.Close()
	require.Error(t, nilResult.WritePayload(context.Background(), io.Discard))
	require.Error(t, r.WritePayload(context.Background(), nil))
}
func TestResultCancellationAndExpiryNeverReachWriter(t *testing.T) {
	for _, mode := range []string{"during-read", "after-read", "during-resolve", "expiry", "credential", "lifetime"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolution := fixtureResolution()
			clock := time.Now()
			calls := 0
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				calls++
				if mode == "during-resolve" && calls == 3 {
					cancel()
				}
				return resolution, nil
			})
			a.now = func() time.Time { return clock }
			r, err := a.CaptureResult(ctx, resultRequest(), func(context.Context) ([]byte, error) {
				if mode == "during-read" {
					cancel()
				}
				return []byte(`{"record":{}}`), nil
			})
			if mode == "during-read" {
				require.Error(t, err)
				require.Nil(t, r)
				return
			}
			require.NoError(t, err)
			defer r.Close()
			var out bytes.Buffer
			switch mode {
			case "after-read", "lifetime":
				cancel()
			case "expiry":
				clock = clock.Add(2 * time.Minute)
			case "credential":
				deadlineCtx, stop := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer stop()
				ctx = deadlineCtx
			}
			require.Error(t, r.WritePayload(ctx, &out))
			require.Empty(t, out.Bytes())
			assertResultPurged(t, r)
		})
	}
}
func TestResultGrantChangesCannotRenewOriginalAuthority(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	resolution.Grants = []Grant{fixtureGrant(resolution.ServerTime)}
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
	r := captureFixture(t, a, context.Background())
	resolution.Grants[0].Revision++
	var out bytes.Buffer
	require.ErrorContains(t, r.WritePayload(context.Background(), &out), "RESULT_AUTHORITY_CHANGED")
	require.Empty(t, out.Bytes())
	assertResultPurged(t, r)
	// Post-I/O rechecks also refuse a replacement grant before returning a result.
	_, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) {
		resolution.Grants[0].ID = "replacement"
		return []byte(`{}`), nil
	})
	require.Error(t, err)
}
func TestResultOriginalDeadlineAndContextClamp(t *testing.T) {
	resolution := fixtureResolution()
	clock := time.Now()
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
	a.now = func() time.Time { return clock }
	ctx, cancel := context.WithDeadline(context.Background(), clock.Add(20*time.Second))
	defer cancel()
	r := captureFixture(t, a, ctx)
	original := r.authorization.deadline
	resolution.ServerTime = resolution.ServerTime.Add(10 * time.Second)
	resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
	clock = clock.Add(10 * time.Second)
	var out bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &out))
	require.Equal(t, original, r.authorization.deadline)
	envelope, err := DecodeResultEnvelope(out.Bytes())
	require.NoError(t, err)
	require.LessOrEqual(t, envelope.Lease.ExpiresAt.Sub(envelope.Lease.ServerTime), 20*time.Second)
	clock = clock.Add(11 * time.Second)
	require.Error(t, r.WritePayload(context.Background(), io.Discard))
}
func TestResultImportRejectsBindingReplayAndMissingMetadata(t *testing.T) {
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	envelope, started := envelopeFixture(t, a)
	mutations := []func(*ResultEnvelope){
		func(e *ResultEnvelope) { e.Lease.Version = 0 }, func(e *ResultEnvelope) { e.Lease.Subject.ID = "other" }, func(e *ResultEnvelope) { e.Lease.Connection.Space = "other" }, func(e *ResultEnvelope) { e.Lease.Connection.ID = "other" }, func(e *ResultEnvelope) { e.Lease.Connection.Fingerprint.EngineIdentity = "other" }, func(e *ResultEnvelope) { e.Lease.Nonce = "other" }, func(e *ResultEnvelope) { e.Lease.RequestSHA256 = hashBytes([]byte("other")) }, func(e *ResultEnvelope) { e.Lease.GrantSetDigest = "bad" }, func(e *ResultEnvelope) { e.Lease.PayloadSHA256 = "bad" }, func(e *ResultEnvelope) { e.Lease.PayloadSHA256 = hashBytes([]byte("other")) }, func(e *ResultEnvelope) { e.Lease.ServerTime = time.Time{} }, func(e *ResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime }, func(e *ResultEnvelope) { e.Lease.ExpiresAt = e.Lease.ServerTime.Add(2 * time.Minute) }, func(e *ResultEnvelope) { e.Payload = []byte(`[]`) }, func(e *ResultEnvelope) { e.Lease.GrantSetDigest = hashBytes([]byte("other")) },
	}
	for i, mutate := range mutations {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			copy := envelope
			copy.Payload = bytes.Clone(envelope.Payload)
			mutate(&copy)
			r, err := a.ImportResult(context.Background(), resultRequest(), started, copy)
			require.Error(t, err)
			require.Nil(t, r)
		})
	}
	for _, start := range []time.Time{{}, time.Now().Add(time.Hour), time.Now().Add(-2 * time.Minute)} {
		_, err := a.ImportResult(context.Background(), resultRequest(), start, envelope)
		require.Error(t, err)
	}
	_, err := a.ImportResult(context.Background(), ResultRequest{}, started, envelope)
	require.Error(t, err)
	var nilAuthority *Authority
	_, err = nilAuthority.ImportResult(context.Background(), resultRequest(), started, envelope)
	require.Error(t, err)
	_, err = a.ImportResult(context.Background(), resultRequest(), started, ResultEnvelope{})
	require.Error(t, err)
}
func TestResultStrictEnvelopeAndPayload(t *testing.T) {
	for _, value := range []string{"", `{`, `[]`, `{"payload":{},"lease":{"version":1},"unknown":true}`, `{"payload":{},"lease":{"version":1,"unknown":true}}`, `{"payload":{},"lease":{"version":1,"version":1}}`, `{"payload":{"record":{"id":1,"id":2}},"lease":{"version":1}}`, `{"payload":{},"lease":{"version":1}} {}`, `{"payload":{},"lease":{"version":2}}`, `{"lease":{"version":1}}`} {
		_, err := DecodeResultEnvelope([]byte(value))
		require.Error(t, err, value)
	}
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	for _, value := range []string{"", `{`, `[]`, `null`, `{"a":1,"a":2}`} {
		_, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return []byte(value), nil })
		require.Error(t, err)
	}
	r, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return []byte(`{"large":1e99999}`), nil })
	require.NoError(t, err)
	r.Close()
	for _, request := range []ResultRequest{{}, {Nonce: "short", RequestSHA256: hashBytes(nil)}, {Nonce: strings.Repeat("x", 257), RequestSHA256: hashBytes(nil)}, {Nonce: " valid-result-nonce", RequestSHA256: hashBytes(nil)}, {Nonce: "valid-result-nonce", RequestSHA256: strings.Repeat("A", 64)}, {Nonce: "valid-result-nonce", RequestSHA256: strings.Repeat("z", 64)}} {
		_, err := a.CaptureResult(context.Background(), request, func(context.Context) ([]byte, error) { return []byte(`{}`), nil })
		require.Error(t, err)
	}
	_, err = a.CaptureResult(context.Background(), resultRequest(), nil)
	require.Error(t, err)
	var nilAuthority *Authority
	_, err = nilAuthority.CaptureResult(context.Background(), resultRequest(), nil)
	require.Error(t, err)
	_, err = a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return nil, errors.New("read failed") })
	require.ErrorContains(t, err, "read failed")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = a.CaptureResult(ctx, resultRequest(), func(context.Context) ([]byte, error) { t.Fatal("dispatched"); return nil, nil })
	require.Error(t, err)
}

type resultWriter func([]byte) (int, error)

func (w resultWriter) Write(p []byte) (int, error) { return w(p) }
func TestResultWriterErrorsAndPurge(t *testing.T) {
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	r := captureFixture(t, a, context.Background())
	require.ErrorIs(t, r.WritePayload(context.Background(), resultWriter(func([]byte) (int, error) { return 0, nil })), io.ErrShortWrite)
	failure := errors.New("writer failed")
	require.ErrorIs(t, r.WritePayload(context.Background(), resultWriter(func([]byte) (int, error) { return 0, failure })), failure)
	ctx, cancel := context.WithCancel(context.Background())
	r = captureFixture(t, a, ctx)
	cancel()
	require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.payload == nil }, time.Second, time.Millisecond)
}
func TestResultTimersPurgeWithoutRelease(t *testing.T) {
	resolution := fixtureResolution()
	resolution.ValidUntil = resolution.ServerTime.Add(30 * time.Millisecond)
	a := testAuthority(t, fixedResolver(resolution))
	r := captureFixture(t, a, context.Background())
	require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.payload == nil }, time.Second, time.Millisecond)
}
func TestResultDecodeNestedPrimitives(t *testing.T) {
	var value any
	require.NoError(t, strictJSON([]byte(`{"array":[1,true,null,{"a":"x"}]}`), &value))
	var e ResultEnvelope
	require.Error(t, json.Unmarshal([]byte(`{"lease":{"version":"1"}}`), &e))
}

func TestResultDigestOrderingAndUnserializableAuthority(t *testing.T) {
	resolution := fixtureResolution()
	first := fixtureGrant(resolution.ServerTime)
	second := first
	second.ID = "grant-2"
	resolution.Grants = []Grant{first, second}
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
	r := captureFixture(t, a, context.Background())
	resolution.Grants = []Grant{second, first}
	require.NoError(t, r.WritePayload(context.Background(), io.Discard))
	resolution.Grants[0].Decision = &Decision{By: fixtureSubject(), At: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}
	_, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { t.Fatal("dispatched unsupported authority"); return nil, nil })
	require.Error(t, err)
}
func TestResultLastCheckAfterSerialization(t *testing.T) {
	t.Run("lifetime", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		a := testAuthority(t, fixedResolver(fixtureResolution()))
		r := captureFixture(t, a, ctx)
		calls := 0
		a.now = func() time.Time {
			calls++
			if calls == 4 {
				cancel()
			}
			return time.Now()
		}
		var out bytes.Buffer
		require.Error(t, r.WriteEnvelope(context.Background(), &out))
		require.Empty(t, out.Bytes())
		assertResultPurged(t, r)
	})
	for _, envelope := range []bool{false, true} {
		t.Run(map[bool]string{false: "payload", true: "envelope"}[envelope], func(t *testing.T) {
			a := testAuthority(t, fixedResolver(fixtureResolution()))
			r := captureFixture(t, a, context.Background())
			clock := time.Now()
			calls := 0
			a.now = func() time.Time {
				calls++
				if calls == 5 {
					return clock.Add(2 * time.Minute)
				}
				return clock
			}
			var out bytes.Buffer
			require.Error(t, r.write(context.Background(), &out, envelope))
			require.Empty(t, out.Bytes())
			assertResultPurged(t, r)
		})
	}
	t.Run("serialization-error", func(t *testing.T) {
		resolution := fixtureResolution()
		resolution.ServerTime = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
		a := testAuthority(t, fixedResolver(resolution))
		r := captureFixture(t, a, context.Background())
		var out bytes.Buffer
		require.Error(t, r.WriteEnvelope(context.Background(), &out))
		require.Empty(t, out.Bytes())
		assertResultPurged(t, r)
	})
}

func assertResultPurged(t *testing.T, r *Result) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	require.Nil(t, r.payload)
}

func TestResultPurgeAndCloseDoNotWaitForResolver(t *testing.T) {
	for _, mode := range []string{"expiry", "cancel", "close"} {
		t.Run(mode, func(t *testing.T) {
			resolution := fixtureResolution()
			if mode == "expiry" {
				resolution.ValidUntil = resolution.ServerTime.Add(200 * time.Millisecond)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			entered := make(chan context.Context, 1)
			hold := make(chan struct{})
			stalled := false
			a := testAuthority(t, func(resolving context.Context, _ ResolveRequest) (Resolution, error) {
				if stalled {
					entered <- resolving
					<-hold
				}
				return resolution, nil
			})
			r := captureFixture(t, a, ctx)
			stalled = true
			var out bytes.Buffer
			done := make(chan error, 1)
			go func() { done <- r.WritePayload(context.Background(), &out) }()
			resolving := <-entered
			deadline, ok := resolving.Deadline()
			require.True(t, ok)
			require.False(t, deadline.After(r.authorization.deadline))
			switch mode {
			case "cancel":
				cancel()
			case "close":
				r.Close()
			case "expiry":
				select {
				case <-resolving.Done():
				case <-time.After(time.Second):
					t.Fatal("resolver context not bounded")
				}
			}
			require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.payload == nil }, time.Second, time.Millisecond)
			closed := make(chan struct{})
			go func() { r.Close(); close(closed) }()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("Close blocked on resolver")
			}
			close(hold)
			require.Error(t, <-done)
			require.Empty(t, out.Bytes())
		})
	}
}
func TestResultCreationFreshResolveIsAlreadyDeadlineBound(t *testing.T) {
	for _, mode := range []string{"capture", "import"} {
		t.Run(mode, func(t *testing.T) {
			original := testAuthority(t, fixedResolver(fixtureResolution()))
			envelope, started := envelopeFixture(t, original)
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
			defer cancel()
			calls := 0
			a := testAuthority(t, func(resolving context.Context, _ ResolveRequest) (Resolution, error) {
				calls++
				if mode == "import" || calls == 2 {
					_, ok := resolving.Deadline()
					require.True(t, ok)
					<-resolving.Done()
				}
				return fixtureResolution(), nil
			})
			if mode == "import" {
				r, err := a.ImportResult(ctx, resultRequest(), started, envelope)
				require.Error(t, err)
				require.Nil(t, r)
			} else {
				r, err := a.CaptureResult(ctx, resultRequest(), func(context.Context) ([]byte, error) { return []byte(`{}`), nil })
				require.Error(t, err)
				require.Nil(t, r)
			}
		})
	}
}

func TestResultFreshNarrowingAndCloseBetweenReleasePhases(t *testing.T) {
	resolution := fixtureResolution()
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
	r := captureFixture(t, a, context.Background())
	original := r.authorization.deadline
	resolution.ValidUntil = resolution.ServerTime.Add(10 * time.Second)
	require.NoError(t, r.WritePayload(context.Background(), io.Discard))
	require.True(t, r.authorization.deadline.Before(original))
	require.NoError(t, r.guard(context.Background()))
	r.Close()
	var out bytes.Buffer
	require.Error(t, r.writeAuthorized(context.Background(), &out, false))
	require.Empty(t, out.Bytes())
}

func TestResultCancellationBetweenResolveAndWriter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	r := captureFixture(t, a, ctx)
	require.NoError(t, r.guard(context.Background()))
	// Hold the asynchronous purge callback to exercise cancellation at the
	// synchronous final handoff, independently of whichever goroutine wins.
	require.True(t, r.stopLifetime())
	cancel()
	var out bytes.Buffer
	require.Error(t, r.writeAuthorized(context.Background(), &out, true))
	require.Empty(t, out.Bytes())
	assertResultPurged(t, r)
}

func TestResultTransferredBuffersClearBeforeFreshResolve(t *testing.T) {
	input := []byte(`{"record":{"name":"Ada"}}`)
	calls := 0
	a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
		calls++
		if calls == 2 {
			require.Equal(t, make([]byte, len(input)), input)
		}
		return fixtureResolution(), nil
	})
	r, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return input, nil })
	require.NoError(t, err)
	defer r.Close()
	var out bytes.Buffer
	require.NoError(t, r.WriteEnvelope(context.Background(), &out))
	envelope, err := DecodeResultEnvelope(out.Bytes())
	require.NoError(t, err)
	a.resolve = func(context.Context, ResolveRequest) (Resolution, error) {
		require.Equal(t, make([]byte, len(envelope.Payload)), []byte(envelope.Payload))
		return fixtureResolution(), nil
	}
	imported, err := a.ImportResult(context.Background(), resultRequest(), time.Now(), envelope)
	require.NoError(t, err)
	defer imported.Close()
	denied := []byte(`{"secret":"discard"}`)
	_, err = a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) { return denied, errors.New("failed after buffering") })
	require.Error(t, err)
	require.Equal(t, make([]byte, len(denied)), denied)
}

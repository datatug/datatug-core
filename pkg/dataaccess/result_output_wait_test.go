package dataaccess

import (
	"bytes"
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func outputWaitResult(t *testing.T, ctx context.Context, table bool, resolver Resolver) *Result {
	t.Helper()
	a := testAuthority(t, resolver)
	if table {
		return previewResult(t, a, ctx, previewOperation(), newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}}))
	}
	return captureFixture(t, a, ctx)
}

func TestResultStalledResolverDoesNotSerializeOtherDenials(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, envelope := range []bool{false, true} {
			for _, boundary := range []string{"close", "cancel", "credential-deadline"} {
				t.Run(map[bool]string{false: "v1", true: "table"}[table]+"/"+map[bool]string{false: "payload", true: "envelope"}[envelope]+"/"+boundary, func(t *testing.T) {
					lifetime, cancel := context.WithCancel(context.Background())
					defer cancel()
					if boundary == "credential-deadline" {
						lifetime, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
						defer cancel()
					}
					resolution := fixtureResolution()
					if table {
						resolution = qualifiedResolution()
					}
					entered := make(chan struct{}, 1)
					unblock := make(chan struct{})
					var once sync.Once
					release := func() { once.Do(func() { close(unblock) }) }
					defer release()
					var armed atomic.Bool
					resolver := func(context.Context, ResolveRequest) (Resolution, error) {
						if armed.Load() {
							entered <- struct{}{}
							<-unblock
						}
						return resolution, nil
					}
					r := outputWaitResult(t, lifetime, table, resolver)
					defer r.Close()
					armed.Store(true)
					var first bytes.Buffer
					done := make(chan error, 1)
					go func() {
						if envelope {
							done <- r.WriteEnvelope(context.Background(), &first)
						} else {
							done <- r.WritePayload(context.Background(), &first)
						}
					}()
					select {
					case <-entered:
					case <-time.After(time.Second):
						t.Fatal("first writer did not enter fresh resolver")
					}
					switch boundary {
					case "close":
						r.Close()
					case "cancel":
						cancel()
					case "credential-deadline":
						<-lifetime.Done()
					}
					denied := make(chan error, 1)
					var second bytes.Buffer
					go func() { denied <- r.WritePayload(context.Background(), &second) }()
					select {
					case err := <-denied:
						require.Error(t, err)
						require.Empty(t, second.Bytes())
					case <-time.After(150 * time.Millisecond):
						t.Fatal("denied output waited for unrelated resolver I/O")
					}
					release()
					select {
					case err := <-done:
						require.Error(t, err)
					case <-time.After(time.Second):
						t.Fatal("first resolver did not unwind")
					}
					require.Empty(t, first.Bytes())
				})
			}
		}
	}
}

func TestResultQueuedOutputCancellationAndFreshReauthorization(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, envelope := range []bool{false, true} {
			for _, boundary := range []string{"caller-cancel", "caller-deadline", "close", "lifetime-cancel", "lease-deadline", "success", "revoked"} {
				t.Run(map[bool]string{false: "v1", true: "table"}[table]+"/"+map[bool]string{false: "payload", true: "envelope"}[envelope]+"/"+boundary, func(t *testing.T) {
					lifetime, cancel := context.WithCancel(context.Background())
					defer cancel()
					resolution := fixtureResolution()
					if table {
						resolution = qualifiedResolution()
					}
					if boundary == "lease-deadline" {
						resolution.ValidUntil = resolution.ServerTime.Add(100 * time.Millisecond)
					}
					var armed, revoked atomic.Bool
					var calls atomic.Int32
					r := outputWaitResult(t, lifetime, table, func(context.Context, ResolveRequest) (Resolution, error) {
						current := resolution
						if armed.Load() {
							calls.Add(1)
						}
						if revoked.Load() {
							current.Protection.ReadPosture = ReadNone
							current.Grants = nil
						}
						return current, nil
					})
					defer r.Close()
					writer := &stalledResultWriter{entered: make(chan []byte, 1), release: make(chan struct{})}
					var once sync.Once
					release := func() { once.Do(func() { close(writer.release) }) }
					defer release()
					firstDone := make(chan error, 1)
					go func() {
						if envelope {
							firstDone <- r.WriteEnvelope(context.Background(), writer)
						} else {
							firstDone <- r.WritePayload(context.Background(), writer)
						}
					}()
					select {
					case <-writer.entered:
					case <-time.After(time.Second):
						t.Fatal("first output did not enter writer")
					}
					armed.Store(true)
					request, cancelRequest := context.WithCancel(context.Background())
					defer cancelRequest()
					if boundary == "caller-deadline" {
						request, cancelRequest = context.WithTimeout(context.Background(), 100*time.Millisecond)
						defer cancelRequest()
					}
					var second bytes.Buffer
					secondDone := make(chan error, 1)
					go func() {
						if envelope {
							secondDone <- r.WriteEnvelope(request, &second)
						} else {
							secondDone <- r.WritePayload(request, &second)
						}
					}()
					require.Eventually(t, func() bool { return calls.Load() > 0 }, time.Second, time.Millisecond, "queued writer must not own authority I/O serialization")
					// This observation occurs while the first external writer is still blocked.
					select {
					case err := <-secondDone:
						t.Fatalf("second output escaped serialization: %v", err)
					case <-time.After(5 * time.Millisecond):
					}
					switch boundary {
					case "caller-cancel":
						cancelRequest()
					case "caller-deadline":
						<-request.Done()
					case "close":
						r.Close()
					case "lifetime-cancel":
						cancel()
					case "lease-deadline":
						require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.payload == nil }, time.Second, time.Millisecond)
					case "success", "revoked":
						if boundary == "revoked" {
							revoked.Store(true)
						}
						release()
					}
					select {
					case err := <-secondDone:
						if boundary == "success" {
							require.NoError(t, err)
							require.NotEmpty(t, second.Bytes())
						} else {
							require.Error(t, err)
							require.Empty(t, second.Bytes())
						}
					case <-time.After(150 * time.Millisecond):
						t.Fatal("queued output did not observe completion, cancellation or purge")
					}
					if boundary == "success" || boundary == "revoked" {
						require.GreaterOrEqual(t, calls.Load(), int32(2), "completion must cause a new fresh resolve before output")
					}
					release()
					select {
					case err := <-firstDone:
						require.NoError(t, err, "already authorized output remains outside recall")
					case <-time.After(time.Second):
						t.Fatal("first writer did not finish")
					}
				})
			}
		}
	}
}

func TestResultOutputZeroValueAndRepeatedClose(t *testing.T) {
	var absent *Result
	require.Error(t, absent.WritePayload(context.Background(), &bytes.Buffer{}))
	zero := new(Result)
	zero.Close()
	zero.Close()
	require.Error(t, zero.WritePayload(context.Background(), &bytes.Buffer{}))
	require.Error(t, zero.WriteEnvelope(context.Background(), nil))
}

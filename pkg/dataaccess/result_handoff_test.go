package dataaccess

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The test observes an alias of the in-flight argument only to verify ownership
// and clearing. A production io.Writer must not retain this argument on return.
type stalledResultWriter struct {
	entered chan []byte
	release chan struct{}
	copied  []byte
}

func (w *stalledResultWriter) Write(data []byte) (int, error) {
	w.entered <- data
	<-w.release
	w.copied = bytes.Clone(data)
	return len(data), nil
}

func TestResultDetachedOutputDoesNotBlockPurgeOrNativeCleanup(t *testing.T) {
	for _, table := range []bool{false, true} {
		for _, envelope := range []bool{false, true} {
			for _, mode := range []string{"close", "cancel", "deadline"} {
				t.Run(map[bool]string{false: "v1", true: "table"}[table]+"/"+map[bool]string{false: "payload", true: "envelope"}[envelope]+"/"+mode, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()
					resolution := fixtureResolution()
					if table {
						resolution = qualifiedResolution()
					}
					if mode == "deadline" {
						resolution.ValidUntil = resolution.ServerTime.Add(100 * time.Millisecond)
					}
					a := testAuthority(t, fixedResolver(resolution))
					var r *Result
					var span *previewSpan
					if table {
						span = newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
						r = previewResult(t, a, ctx, previewOperation(), span)
					} else {
						r = captureFixture(t, a, ctx)
					}
					retained := r.payload
					writer := &stalledResultWriter{entered: make(chan []byte, 1), release: make(chan struct{})}
					released := false
					defer func() {
						if !released {
							close(writer.release)
						}
					}()
					done := make(chan error, 1)
					go func() {
						if envelope {
							done <- r.WriteEnvelope(context.Background(), writer)
						} else {
							done <- r.WritePayload(context.Background(), writer)
						}
					}()
					var flight []byte
					select {
					case flight = <-writer.entered:
					case <-time.After(time.Second):
						t.Fatal("writer did not enter")
					}
					expected := bytes.Clone(flight)
					defer clear(expected)
					require.False(t, &retained[0] == &flight[0], "writer must never borrow private retained storage")
					switch mode {
					case "close":
						closed := make(chan struct{})
						go func() { r.Close(); close(closed) }()
						select {
						case <-closed:
						case <-time.After(100 * time.Millisecond):
							t.Fatal("Close waited for output I/O")
						}
					case "cancel":
						cancel()
					}
					require.Eventually(t, func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.payload == nil }, time.Second, time.Millisecond)
					require.Equal(t, make([]byte, len(retained)), retained)
					require.Equal(t, expected, flight, "an in-flight writer still owns stable input")
					if span != nil {
						require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
					}
					close(writer.release)
					released = true
					require.NoError(t, <-done)
					require.Equal(t, expected, writer.copied, "already authorized output remains outside recall")
					require.Equal(t, make([]byte, len(flight)), flight, "output scratch must clear after Write returns")
				})
			}
		}
	}
}

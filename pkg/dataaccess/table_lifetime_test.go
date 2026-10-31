package dataaccess

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTableResultOriginalGrantAExpiresDespiteLongerB(t *testing.T) {
	resolution := qualifiedResolution()
	resolution.ServerTime = time.Now()
	resolution.ValidUntil = resolution.ServerTime.Add(time.Minute)
	resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(100 * time.Millisecond)
	a := testAuthority(t, fixedResolver(resolution))
	span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
	r := previewResult(t, a, context.Background(), previewOperation(), span)
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
	assertResultPurged(t, r)
	var out bytes.Buffer
	require.Error(t, r.WritePayload(context.Background(), &out))
	require.Empty(t, out.Bytes())
}

func TestTableResultFreshShorterCeilingPurgesBeforeUncooperativeFactReturns(t *testing.T) {
	resolution := qualifiedResolution()
	var shorten atomic.Bool
	fresh := func(context.Context, ResolveRequest) (Resolution, error) {
		copy := resolution
		if shorten.Load() {
			copy.ServerTime = time.Now()
			copy.ValidUntil = copy.ServerTime.Add(70 * time.Millisecond)
		}
		return copy, nil
	}
	a := testAuthority(t, fresh)
	span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
	r := previewResult(t, a, context.Background(), previewOperation(), span)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	span.verify = func(ctx context.Context) error { once.Do(func() { close(entered) }); <-release; return ctx.Err() }
	shorten.Store(true)
	done := make(chan error, 1)
	var out bytes.Buffer
	go func() { done <- r.WriteEnvelope(context.Background(), &out) }()
	<-entered
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
	assertResultPurged(t, r)
	close(release)
	require.Error(t, <-done)
	require.Empty(t, out.Bytes())
}

func TestTableResultFreshControlAndNativeFactChangesDeny(t *testing.T) {
	for _, mode := range []string{"membership", "outage", "protection", "source", "native-name", "native-source", "native-closed", "context"} {
		t.Run(mode, func(t *testing.T) {
			resolution := qualifiedResolution()
			var unavailable bool
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				if unavailable {
					return Resolution{}, errors.New("secret")
				}
				return resolution, nil
			})
			span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r := previewResult(t, a, ctx, previewOperation(), span)
			switch mode {
			case "membership":
				resolution.Subject.ID = "removed"
			case "outage":
				unavailable = true
			case "protection":
				resolution.Protection.Revision++
			case "source":
				resolution.Connection.Fingerprint.Database = "other"
			case "native-name":
				span.facts.Table.Name = "other"
			case "native-source":
				span.facts.Connection.Fingerprint.Database = "other"
			case "native-closed":
				span.closed.Store(true)
			case "context":
				cancel()
			}
			var out bytes.Buffer
			err := r.WritePayload(context.Background(), &out)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
			require.Empty(t, out.Bytes())
			assertResultPurged(t, r)
		})
	}
}

func TestTableLateOpenCancellationOwnsLateSpanWithoutDataDispatch(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	ctx, cancel := context.WithCancel(context.Background())
	entered := make(chan struct{})
	release := make(chan struct{})
	adapter := &previewAdapter{}
	span := newPreviewSpan(adapter)
	source := tableSourceFunc(func(context.Context, TableIdentityRequest) (TableReadSpan, error) {
		close(entered)
		<-release
		return span, nil
	})
	done := make(chan error, 1)
	go func() {
		r, err := a.ReadTableResult(ctx, resultRequest(), previewOperation(), source)
		if r != nil {
			r.Close()
		}
		done <- err
	}()
	<-entered
	cancel()
	close(release)
	require.Error(t, <-done)
	require.Eventually(t, span.closed.Load, time.Second, time.Millisecond)
	require.Zero(t, adapter.calls.Load())
	var nilCtx context.Context
	_, err := a.ReadTableResult(nilCtx, resultRequest(), previewOperation(), source)
	require.Error(t, err)
}

func TestTableSpanCleanupAfterOutputCannotRetractOrAppend(t *testing.T) {
	a := testAuthority(t, fixedResolver(qualifiedResolution()))
	span := newPreviewSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"}, []any{int64(1)})}})
	report := make(chan struct{})
	release := make(chan struct{})
	span.closeErr = errors.New("owned cleanup failure")
	reported := make(chan error, 1)
	span.stop = func() { close(report); <-release; reported <- span.closeErr }
	r := previewResult(t, a, context.Background(), previewOperation(), span)
	var out bytes.Buffer
	require.NoError(t, r.WritePayload(context.Background(), &out))
	before := bytes.Clone(out.Bytes())
	r.Close()
	assertResultPurged(t, r)
	<-report
	require.Equal(t, before, out.Bytes())
	close(release)
	require.Error(t, <-reported)
	require.Error(t, r.WritePayload(context.Background(), &out))
	require.Equal(t, before, out.Bytes())
}

package meaninggraph

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

type failedRetainedBody struct{}

func (failedRetainedBody) Read([]byte) (int, error) {
	return 0, errors.New("retained body interrupted")
}
func (failedRetainedBody) Close() error { return nil }

type cancelRetainedBody struct{ cancel context.CancelFunc }

func (b cancelRetainedBody) Read([]byte) (int, error) { b.cancel(); return 0, io.EOF }
func (cancelRetainedBody) Close() error               { return nil }

func TestResolverTransportAndResourceFailures(t *testing.T) {
	f, c := newFixture(t)
	c.Limits.MaxFiles = -1
	_, err := NewResolver(c)
	require.Error(t, err)
	c.Limits = Limits{}
	r, err := NewResolver(c)
	require.NoError(t, err)
	_, err = r.Resolve(context.Background(), datatug.DataModelReference{})
	require.Error(t, err)
	for _, p := range []string{"", "/absolute", `a\b`, strings.Repeat("a", 2049), "a\x00b", "a\x7fb", "a//b", "a/../b", "./a"} {
		require.Error(t, safePath(p), p)
	}
	s := &session{resolver: r, cache: map[string][]byte{}}
	_, err = s.read(context.Background(), "../outside", "")
	require.Error(t, err)
	_, err = s.read(context.Background(), "catalog.json", "invalid")
	require.Error(t, err)
	putBytes(t, f.root, "one", []byte("1"))
	putBytes(t, f.root, "two", []byte("2"))
	c.Limits = Limits{MaxFileBytes: 1, MaxTotalBytes: 1}
	r, err = NewResolver(c)
	require.NoError(t, err)
	s = &session{resolver: r, cache: map[string][]byte{}}
	_, err = s.read(context.Background(), "one", "")
	require.NoError(t, err)
	_, err = s.read(context.Background(), "two", "")
	require.ErrorContains(t, err, "byte bound")
	c.Limits = Limits{}
	c.FixtureRoot = ""
	r, err = NewResolver(c)
	require.NoError(t, err)
	s = &session{resolver: r, cache: map[string][]byte{}}
	_, err = s.read(context.Background(), "%", "")
	require.Error(t, err)
	r.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: failedRetainedBody{}, Header: make(http.Header), Request: req}, nil
	})
	_, err = s.read(context.Background(), "catalog.json", "")
	require.ErrorContains(t, err, "interrupted")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: cancelRetainedBody{cancel}, Header: make(http.Header), Request: req}, nil
	})
	_, err = s.read(ctx, "catalog.json", "")
	require.ErrorIs(t, err, context.Canceled)
}

//go:build unix

package meaninggraph

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFixtureRootPathResolutionFailureReturnsNoHandle(t *testing.T) {
	previous := fixtureAbsolutePath
	t.Cleanup(func() { fixtureAbsolutePath = previous })
	want := errors.New("root path unavailable")
	fixtureAbsolutePath = func(string) (string, error) { return "", want }
	fd, err := openFixtureRoot("/")
	if fd != -1 || !errors.Is(err, want) {
		t.Fatal("root descriptor failure lost", fd, err)
	}
}

func TestResolverFixtureRefusesSymlinksAndNonregularInputs(t *testing.T) {
	f, c := newFixture(t)
	r, err := NewResolver(c)
	require.NoError(t, err)
	source := filepath.Join(f.root, "catalog.json")
	outside := filepath.Join(t.TempDir(), "catalog.json")
	body, err := os.ReadFile(source)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(outside, body, 0600))
	require.NoError(t, os.Remove(source))
	require.NoError(t, os.Symlink(outside, source))
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err)
	require.NoError(t, os.Remove(source))
	require.NoError(t, unix.Mkfifo(source, 0600))
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err)
	rootLink := filepath.Join(t.TempDir(), "root-link")
	require.NoError(t, os.Symlink(f.root, rootLink))
	c.FixtureRoot = filepath.Join(rootLink, "child")
	require.NoError(t, os.Mkdir(filepath.Join(f.root, "child"), 0700))
	_, err = NewResolver(c)
	require.Error(t, err)
	c.FixtureRoot = rootLink
	_, err = NewResolver(c)
	require.Error(t, err)
	f, c = newFixture(t)
	r, err = NewResolver(c)
	require.NoError(t, err)
	require.NoError(t, os.RemoveAll(filepath.Join(f.root, "revisions")))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(f.root, "revisions")))
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err)
}

func TestResolverFixtureOSResourceFailures(t *testing.T) {
	if scenario := os.Getenv("MG_FIXTURE_OS_FAILURE"); scenario != "" {
		switch scenario {
		case "cwd removed":
			previous, err := os.Getwd()
			require.NoError(t, err)
			dir := t.TempDir()
			require.NoError(t, os.Chdir(dir))
			require.NoError(t, os.Remove(dir))
			_, failure := openFixtureRoot(".")
			require.NoError(t, os.Chdir(previous))
			require.Error(t, failure)
		case "descriptor limit":
			var old unix.Rlimit
			require.NoError(t, unix.Getrlimit(unix.RLIMIT_NOFILE, &old))
			limit := old
			limit.Cur = 0
			require.NoError(t, unix.Setrlimit(unix.RLIMIT_NOFILE, &limit))
			_, failure := openFixtureRoot("/")
			require.NoError(t, unix.Setrlimit(unix.RLIMIT_NOFILE, &old))
			require.Error(t, failure)
		default:
			t.Fatal("unknown failure scenario")
		}
		return
	}
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, scenario := range []string{"cwd removed", "descriptor limit"} {
		args := []string{"-test.run=^TestResolverFixtureOSResourceFailures$"}
		// Go's coverage runtime merges child executions of this same test
		// binary when they write their actual counters into its GOCOVERDIR.
		if dir := os.Getenv("GOCOVERDIR"); testing.CoverMode() != "" && dir != "" {
			args = append(args, "-test.gocoverdir="+dir)
		}
		cmd := exec.Command(executable, args...)
		cmd.Env = append(os.Environ(), "MG_FIXTURE_OS_FAILURE="+scenario)
		body, err := cmd.CombinedOutput()
		require.NoError(t, err, string(body))
	}
	fd, err := openFixtureRoot("/")
	require.NoError(t, err)
	require.NoError(t, unix.Close(fd))
	_, err = openFixtureFile(filepath.Join(t.TempDir(), "absent"), "file")
	require.Error(t, err)
	file, err := os.CreateTemp(t.TempDir(), "closed")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	_, err = regularFixtureFile(file)
	require.Error(t, err)
}

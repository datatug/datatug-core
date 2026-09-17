package dalgostore_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// forbiddenDriverPackages are the DALgo drivers datatug-core must never
// depend on (dalgo-project-store#ac:core-depends-on-dalgo-interfaces-only):
// the project store above the DALgo boundary must not know which concrete
// backend it runs on. A match is a whole import path or a "/"-delimited path
// segment equal to one of these names, never a substring: that rules out a
// false positive against an unrelated package whose name merely contains
// one of these as a substring, and a false negative is impossible because
// dalgo2ingitdb and dalgo2ingitdb4github are themselves distinct segments
// (the "4github" suffix keeps them from ever sharing a segment).
var forbiddenDriverPackages = []string{
	"dalgo2ingitdb",
	"dalgo2ingitdb4github",
	"dalgo2openvaultdb",
}

// TestModuleDependencies_NoDalgoDriver runs `go list -deps ./...` over the
// whole datatug-core module (not just this package) and asserts none of the
// forbidden driver packages appear in the dependency graph. `go list -deps`
// without -test excludes test-only imports, so this package's own use of the
// dalgo2memory test double (a subpackage of the already-required
// dal-go/dalgo module, not a driver) does not affect the result.
func TestModuleDependencies_NoDalgoDriver(t *testing.T) {
	moduleRoot := moduleRootDir(t)

	out := runGoCommand(t, moduleRoot, "list", "-deps", "./...")
	for _, importPath := range strings.Split(strings.TrimSpace(out), "\n") {
		importPath = strings.TrimSpace(importPath)
		if importPath == "" {
			continue
		}
		for _, forbidden := range forbiddenDriverPackages {
			require.False(t, importPath == forbidden || hasPathSegment(importPath, forbidden),
				"datatug-core must not depend on the DALgo driver package %q above the DALgo boundary (found %q)", forbidden, importPath)
		}
	}
}

// hasPathSegment reports whether importPath contains segment as one whole
// "/"-delimited component, e.g. "github.com/ingitdb/dalgo2ingitdb" contains
// "dalgo2ingitdb", but "github.com/x/dalgo2ingitdbextra" does not.
func hasPathSegment(importPath, segment string) bool {
	for _, part := range strings.Split(importPath, "/") {
		if part == segment {
			return true
		}
	}
	return false
}

// goBin resolves the go binary this test itself runs under. runtime.GOROOT()
// is deliberately not used here: it is deprecated since Go 1.24 for exactly
// this purpose (SA1019) — it names the root the binary was built with, not
// the one it is running under, so it can be wrong once a binary is copied
// between machines. The deprecation notice's own recommendation is what this
// does instead: locate "go" via the system PATH, the same PATH `go test`
// itself was invoked from, so this resolves to that same toolchain.
func goBin(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	require.NoError(t, err, "go binary not found on PATH")
	return path
}

// runGoCommand runs the go toolchain's binary with args in dir, with
// GOTOOLCHAIN pinned to the toolchain already running this test (so go.mod's
// toolchain directive cannot trigger a network fetch of a different one) and
// GOFLAGS set to allow go.mod/go.sum adjustments list may want to make.
func runGoCommand(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(goBin(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOTOOLCHAIN=local", "GOFLAGS=-mod=mod")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "go %s failed: %s", strings.Join(args, " "), stderr.String())
	return stdout.String()
}

// moduleRootDir resolves the root of the datatug-core module regardless of
// which directory `go test` was invoked from, by asking the Go toolchain for
// the go.mod that governs the current package's directory.
func moduleRootDir(t *testing.T) string {
	t.Helper()
	out := runGoCommand(t, ".", "env", "GOMOD")
	gomod := strings.TrimSpace(out)
	require.NotEmpty(t, gomod)
	require.NotEqual(t, os.DevNull, gomod, "not inside a Go module")
	return filepath.Dir(gomod)
}

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
// backend it runs on.
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

	cmd := exec.Command("go", "list", "-deps", "./...")
	cmd.Dir = moduleRoot
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	require.NoError(t, err, "go list -deps ./... failed: %s", stderr.String())

	deps := stdout.String()
	for _, forbidden := range forbiddenDriverPackages {
		require.NotContains(t, deps, forbidden,
			"datatug-core must not depend on the DALgo driver package %q above the DALgo boundary", forbidden)
	}
}

// moduleRootDir resolves the root of the datatug-core module regardless of
// which directory `go test` was invoked from, by asking the Go toolchain for
// the go.mod that governs the current package's directory.
func moduleRootDir(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOMOD").Output()
	require.NoError(t, err)
	gomod := strings.TrimSpace(string(out))
	require.NotEmpty(t, gomod)
	require.NotEqual(t, os.DevNull, gomod, "not inside a Go module")
	return filepath.Dir(gomod)
}

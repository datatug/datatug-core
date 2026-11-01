package filestore

import (
	"context"
	"os"
	"os/exec"
	"runtime/debug"
	"strings"
	"testing"
	"time"
)

func TestVisibilityDeepJSONRefusesWithinBoundedStack(t *testing.T) {
	const marker = "DATATUG_TEST_ARCHIVE_DEPTH_CHILD"
	if os.Getenv(marker) == "1" {
		// Bound the child stack, rather than reproduce a process-wide overflow
		// with the runtime's much larger default stack allowance.
		// Leave room for the standard parser's documented nesting limit while
		// keeping a runaway duplicate-key walk far below the runtime default.
		debug.SetMaxStack(16 << 20)
		debug.SetTraceback("single")
		dir := t.TempDir()
		writeVisibilityFixture(t, dir, visibilityFile, `{"x":`+strings.Repeat("[", (2<<20)-5))
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		if rows, err := ReadProjectArchivedItemsFromRoot(root); err == nil || rows != nil {
			t.Fatal("deep malformed archive admitted", rows, err)
		}
		if rows, err := ReadProjectArchivedItems(dir); err == nil || rows != nil {
			t.Fatal("deep malformed legacy archive admitted", rows, err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestVisibilityDeepJSONRefusesWithinBoundedStack$", "-test.count=1")
	command.Env = append(os.Environ(), marker+"=1", "GOMAXPROCS=2", "GOMEMLIMIT=64MiB")
	output, err := command.CombinedOutput()
	if err != nil {
		if len(output) > 4096 {
			output = output[:4096]
		}
		t.Fatalf("bounded archive decoder child failed: %v\n%s", err, output)
	}
}

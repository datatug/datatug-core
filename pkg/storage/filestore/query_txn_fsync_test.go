package filestore

import (
	"bytes"
	"log"
	"path/filepath"
	"strings"
	"testing"
)

// S3: fsyncDirBestEffort's doc comment always claimed "a failure here is
// logged", but the implementation only ever discarded the error
// (`_ = d.Sync()`), with no logging, metric or any other operator
// visibility. These tests prove the function now actually logs a failure
// (via the package's existing log.Printf convention - see utils.go's
// file-close failure logging) and stays silent on success, matching the
// comment for real rather than by luck.
//
// os.Open on a directory that does not exist is used to force a
// deterministic, portable failure - forcing os.File.Sync itself to fail
// is not reliably possible across platforms/filesystems in a unit test,
// but fsyncDirBestEffort treats an Open failure exactly the same as a
// Sync failure (both are "a failure here"), so this exercises the same
// logging path.

func TestFsyncDirBestEffort_LogsOnFailure(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	t.Cleanup(func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	})

	missing := filepath.Join(t.TempDir(), "does-not-exist")
	fsyncDirBestEffort(missing)

	if !strings.Contains(buf.String(), missing) {
		t.Fatalf("expected a log line naming the failed directory %q, got: %q", missing, buf.String())
	}
}

func TestFsyncDirBestEffort_NoLogOnSuccess(t *testing.T) {
	var buf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	fsyncDirBestEffort(t.TempDir())

	if buf.Len() != 0 {
		t.Fatalf("expected no log output when fsyncDirBestEffort succeeds, got: %q", buf.String())
	}
}

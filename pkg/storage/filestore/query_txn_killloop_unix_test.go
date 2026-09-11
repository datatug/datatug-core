//go:build unix

package filestore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// killLoopWriterEnv, when set to a project directory, turns
// TestKillLoopWriterProcess into an endless writer against that project.
const killLoopWriterEnv = "DATATUG_FILESTORE_KILLLOOP_PROJECT"

// TestKillLoopWriterProcess is not a test on its own. The kill-loop test
// re-executes the test binary with only this test selected and
// killLoopWriterEnv set, to get a real writer process it can SIGKILL at an
// arbitrary point of a transaction. Its writes vary the body size (so a
// kill lands in staging, the journal write, the install or the cleanup),
// change the query type every third write (a type change removes the stale
// sidecar in the same transaction) and delete and re-create the query
// every seventh.
func TestKillLoopWriterProcess(t *testing.T) {
	projectDir := os.Getenv(killLoopWriterEnv)
	if projectDir == "" {
		t.Skip("helper process for TestQueryTxn_KillLoopLeavesEveryStateFineOrRecoverable")
	}
	store := newFsQueriesStore(projectDir)
	ctx := context.Background()
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	fail := func(err error) {
		fmt.Fprintln(os.Stderr, "writer:", err)
		os.Exit(3)
	}
	for i := 0; ; i++ {
		stamp := fmt.Sprintf("%d-%d", os.Getpid(), i)
		q := dtqlQuery("q", "", "B"+stamp+strings.Repeat("x", rng.Intn(200_000)))
		q.Title = "T" + stamp
		if i%3 == 1 {
			q.Type = datatug.QueryTypeSQL
		}
		cur, err := store.LoadQueryRevision(ctx, "q")
		switch {
		case err == nil && i%7 == 3:
			if err := store.DeleteQueryRevision(ctx, "q", cur.Revision); err != nil {
				fail(err)
			}
		case err == nil:
			if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: cur.Revision}); err != nil {
				fail(err)
			}
		case errors.Is(err, os.ErrNotExist):
			if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
				fail(err)
			}
		default:
			fail(err)
		}
		if i == 0 {
			fmt.Println("ready")
		}
	}
}

// TestQueryTxn_KillLoopLeavesEveryStateFineOrRecoverable is the review's
// killloop as a regression test: it SIGKILLs a real writer process at
// random points and, after every kill, opens a fresh store and requires
// that it reads one complete, consistent pair (or none, after a delete),
// accepts the next write, and leaves no transaction artifact behind - no
// torn pair, no stale sidecar, no store stuck unable to read or write.
func TestQueryTxn_KillLoopLeavesEveryStateFineOrRecoverable(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns and kills writer processes")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	projectDir := t.TempDir()
	seed := dtqlQuery("q", "", "B0")
	seed.Title = "T0"
	if _, err := newFsQueriesStore(projectDir).PutQuery(context.Background(), &seed, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	const kills = 25
	for i := 0; i < kills; i++ {
		killWriterAtRandom(t, exe, projectDir, time.Duration(rng.Intn(20_000))*time.Microsecond)
		checkKillLoopState(t, projectDir, i)
	}
}

func killWriterAtRandom(t *testing.T, exe, projectDir string, after time.Duration) {
	t.Helper()
	cmd := exec.Command(exe, "-test.run=^TestKillLoopWriterProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), killLoopWriterEnv+"="+projectDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("unexpected error starting the writer: %v", err)
	}
	ready := make(chan bool, 1)
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			if sc.Text() == "ready" {
				ready <- true
				for sc.Scan() { // keep draining until the writer is killed
				}
				return
			}
		}
		ready <- false
	}()
	select {
	case ok := <-ready:
		if !ok {
			_ = cmd.Wait()
			t.Fatalf("the writer exited before its first write: %s", stderr.String())
		}
	case <-time.After(60 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("the writer did not complete its first write within 60s")
	}
	time.Sleep(after)
	_ = cmd.Process.Signal(syscall.SIGKILL)
	_ = cmd.Wait()
	if strings.Contains(stderr.String(), "writer:") {
		t.Fatalf("the writer failed on its own before the kill: %s", stderr.String())
	}
}

func checkKillLoopState(t *testing.T, projectDir string, kill int) {
	t.Helper()
	ctx := context.Background()
	fresh := newFsQueriesStore(projectDir)
	queriesDir := filepath.Join(projectDir, "queries")
	next := dtqlQuery("q", "", "Bcheck")
	next.Title = "Tcheck"

	cur, err := fresh.LoadQueryRevision(ctx, "q")
	switch {
	case errors.Is(err, os.ErrNotExist):
		// Killed between a delete and the re-create: a consistent state.
		if names := queryFileNames(t, queriesDir); len(names) != 0 {
			t.Fatalf("kill %d: no query, but files remain: %v", kill, names)
		}
		if _, err := fresh.PutQuery(ctx, &next, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
			t.Fatalf("kill %d: STUCK - cannot create after recovery: %v", kill, err)
		}
	case err != nil:
		t.Fatalf("kill %d: STUCK - cannot read after recovery: %v", kill, err)
	default:
		title := strings.TrimPrefix(cur.Query.Title, "T")
		stamp := strings.SplitN(strings.TrimPrefix(cur.Query.Text, "B"), "x", 2)[0]
		if title != stamp {
			t.Fatalf("kill %d: TORN pair - title %q, body starts %q", kill, cur.Query.Title, stamp)
		}
		bodyName, err := queryBodyFileName("q", cur.Query.Type)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]bool{"q.query.json": true, bodyName: true}
		names := queryFileNames(t, queriesDir)
		if len(names) != 2 || !want[names[0]] || !want[names[1]] || names[0] == names[1] {
			t.Fatalf("kill %d: expected exactly q.query.json and %s (no stale sidecar), got %v", kill, bodyName, names)
		}
		if _, err := fresh.PutQuery(ctx, &next, datatug.QueryWriteCondition{IfMatch: cur.Revision}); err != nil {
			t.Fatalf("kill %d: STUCK - cannot write after recovery: %v", kill, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(queriesDir, reservedQueryTxnDirName))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "lock" && e.Name() != ".gitignore" {
			t.Fatalf("kill %d: transaction artifact %s left after recovery and a completed write", kill, e.Name())
		}
	}
}

// queryFileNames lists the query files directly under queriesDir.
func queryFileNames(t *testing.T, queriesDir string) []string {
	t.Helper()
	entries, err := os.ReadDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var names []string
	for _, e := range entries {
		if e.Name() != reservedQueryTxnDirName {
			names = append(names, e.Name())
		}
	}
	return names
}

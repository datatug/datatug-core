package filestore

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review S1 and N3 regression tests.

// TestWithQueryReadLock_RereadsUnderTheLockWhenTheNamespaceAppearsMidRead
// proves the S1 mechanism deterministically: a writer's first transaction
// creates the namespace while an unlocked read is in progress, so that
// read's result is discarded and the read runs again under the lock.
func TestWithQueryReadLock_RereadsUnderTheLockWhenTheNamespaceAppearsMidRead(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var guards []queryLockGuard
	err := store.withQueryReadLock(context.Background(), func(g queryLockGuard) error {
		guards = append(guards, g)
		if len(guards) == 1 {
			if _, err := ensureQueryTxnDir(queriesDir); err != nil {
				return err
			}
			return errors.New("whatever the unlocked read saw")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("expected the read under the lock to replace the unlocked result, got: %v", err)
	}
	if len(guards) != 2 {
		t.Fatalf("expected the read to run twice, ran %d times", len(guards))
	}
	if guards[0].txnDir != "" || guards[1].txnDir == "" {
		t.Fatalf("expected an unlocked run then a locked one, got guards %+v", guards)
	}
}

func TestWithQueryReadLock_ReadsOnceWhenNoWriterAppears(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	if err := os.MkdirAll(queriesDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	calls := 0
	if err := store.withQueryReadLock(context.Background(), func(queryLockGuard) error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected one unlocked read, got %d", calls)
	}
	if fileExistsAt(filepath.Join(queriesDir, reservedQueryTxnDirName)) {
		t.Error("expected a read alone never to create the namespace")
	}
}

// TestFirstWriteRace_ReadersNeverSeeATornPair is the review's
// firstwrite-race: a legacy project this store has never written to, read
// by LoadQueries and by the recursive project-tree load while the first
// SaveQuery replaces one of its pairs. Every reader must see the pair
// entirely old or entirely new.
func TestFirstWriteRace_ReadersNeverSeeATornPair(t *testing.T) {
	const pairs, iterations = 400, 8
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	last := fmt.Sprintf("q%04d", pairs-1)
	for it := 0; it < iterations; it++ {
		projectDir := t.TempDir()
		queriesDir := filepath.Join(projectDir, "queries")
		if err := os.MkdirAll(queriesDir, 0o755); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		for k := 0; k < pairs; k++ {
			id := fmt.Sprintf("q%04d", k)
			if err := os.WriteFile(filepath.Join(queriesDir, id+".query.json"), []byte(`{"title":"OLD","type":"DTQL"}`), 0o644); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if err := os.WriteFile(filepath.Join(queriesDir, id+".query.dtql"), []byte("OLD"), 0o644); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		}
		store := newFsQueriesStore(projectDir)
		ctx := context.Background()
		var wg sync.WaitGroup
		var listed, tree *datatug.QueriesFolder
		var listErr, treeErr, saveErr error
		delay := time.Duration(rng.Intn(3000)) * time.Microsecond
		wg.Add(3)
		go func() { defer wg.Done(); listed, listErr = store.LoadQueries(ctx, "") }()
		go func() { defer wg.Done(); tree, treeErr = store.loadQueriesTree(ctx, "") }()
		go func() {
			defer wg.Done()
			time.Sleep(delay)
			q := dtqlQuery(last, "", "NEW")
			q.Title = "NEW"
			saveErr = store.SaveQuery(ctx, &q)
		}()
		wg.Wait()
		if saveErr != nil {
			t.Fatalf("iteration %d: SaveQuery: %v", it, saveErr)
		}
		for reader, result := range map[string]struct {
			folder *datatug.QueriesFolder
			err    error
		}{"LoadQueries": {listed, listErr}, "loadQueriesTree": {tree, treeErr}} {
			if result.err != nil {
				t.Fatalf("iteration %d: %s: %v", it, reader, result.err)
			}
			found := false
			for _, item := range result.folder.Items {
				if item.ID == last {
					found = true
					if item.Title != item.Text {
						t.Errorf("iteration %d: %s saw a TORN pair: title %q, text %q", it, reader, item.Title, item.Text)
					}
				}
			}
			if !found {
				t.Errorf("iteration %d: %s did not return %s", it, reader, last)
			}
		}
	}
}

// TestPutQuery_ResolvesTheLocationAgainUnderTheLock is the N3 regression
// test: the location used to be resolved only before the lock, so a
// symlink swapped into the folder chain while a write waited for the lock
// was followed (MkdirAll) and the pair written outside the queries root.
func TestPutQuery_ResolvesTheLocationAgainUnderTheLock(t *testing.T) {
	skipSymlinksOnWindows(t)
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	seed := dtqlQuery("seed", "", "S")
	if _, err := store.PutQuery(ctx, &seed, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	held, release := make(chan struct{}), make(chan struct{})
	holderDone := make(chan error, 1)
	go func() {
		holderDone <- store.withQueryLock(ctx, func(queryLockGuard) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	putDone := make(chan error, 1)
	go func() {
		q := dtqlQuery("q", "f", "B")
		_, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
		putDone <- err
	}()
	// Let PutQuery pass its pre-lock check (folder "f" does not exist yet,
	// which is valid) and block on the lock, then swap a symlink in.
	time.Sleep(150 * time.Millisecond)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(queriesDir, "f")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	close(release)
	if err := <-holderDone; err != nil {
		t.Fatalf("unexpected error from the lock holder: %v", err)
	}
	if err := <-putDone; !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected the write to be refused once the folder became a symlink, got: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("expected nothing written through the swapped-in symlink, found %v", entries)
	}
}

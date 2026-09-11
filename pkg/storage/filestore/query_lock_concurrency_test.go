package filestore

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/gofrs/flock"
)

// TestQueryLock_BlocksConcurrentAccessAndSeesCompleteResultAfterRelease
// proves the plan's "serialize them with one query-store advisory lock":
// an external holder of the same lock file blocks a concurrent LoadQueries
// call, and once released, the load proceeds and observes the store's
// complete state, never a torn read racing a writer.
func TestQueryLock_BlocksConcurrentAccessAndSeesCompleteResultAfterRelease(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	external := flock.New(filepath.Join(txnDir, "lock"))
	locked, err := external.TryLock()
	if err != nil || !locked {
		t.Fatalf("unexpected error/result taking the external lock: locked=%v err=%v", locked, err)
	}

	done := make(chan struct {
		folder *datatug.QueriesFolder
		err    error
	}, 1)
	go func() {
		folder, err := store.LoadQueries(ctx, "")
		done <- struct {
			folder *datatug.QueriesFolder
			err    error
		}{folder, err}
	}()

	select {
	case <-done:
		t.Fatal("expected LoadQueries to block while an external holder has the lock")
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as expected.
	}

	if err := external.Unlock(); err != nil {
		t.Fatalf("unexpected error releasing the external lock: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("unexpected error after the lock was released: %v", result.err)
		}
		if len(result.folder.Items) != 1 || result.folder.Items[0].Text != "original" {
			t.Fatalf("expected the complete, unchanged record, got: %+v", result.folder.Items)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected LoadQueries to proceed once the external lock was released")
	}
}

// TestQueryLock_BlocksConcurrentProjectLevelRecursiveLoad is N3: the same
// proof as TestQueryLock_BlocksConcurrentAccessAndSeesCompleteResultAfterRelease,
// but for loadQueriesTree - the project-level recursive walk LoadProject
// actually uses (see queries_tree.go) - rather than the single-folder
// LoadQueries. It has its own lock-acquire/recover call
// (loadQueriesTree -> withQueryReadLock) distinct from LoadQueries', so
// this exercises that call site directly instead of only inferring it is
// covered by the single-folder case.
func TestQueryLock_BlocksConcurrentProjectLevelRecursiveLoad(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	external := flock.New(filepath.Join(txnDir, "lock"))
	locked, err := external.TryLock()
	if err != nil || !locked {
		t.Fatalf("unexpected error/result taking the external lock: locked=%v err=%v", locked, err)
	}

	done := make(chan struct {
		folder *datatug.QueriesFolder
		err    error
	}, 1)
	go func() {
		folder, err := store.loadQueriesTree(ctx, "")
		done <- struct {
			folder *datatug.QueriesFolder
			err    error
		}{folder, err}
	}()

	select {
	case <-done:
		t.Fatal("expected loadQueriesTree to block while an external holder has the lock")
	case <-time.After(150 * time.Millisecond):
		// Still blocked, as expected.
	}

	if err := external.Unlock(); err != nil {
		t.Fatalf("unexpected error releasing the external lock: %v", err)
	}

	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("unexpected error after the lock was released: %v", result.err)
		}
		if result.folder == nil || len(result.folder.Items) != 1 || result.folder.Items[0].Text != "original" {
			t.Fatalf("expected the complete, unchanged record, got: %+v", result.folder)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("expected loadQueriesTree to proceed once the external lock was released")
	}
}

// TestQueryLock_CancellationWhileWaitingBehindAHeldLock proves waiting for
// the lock is cancellation-aware even when another holder never releases
// it: PutQuery must return ctx's error promptly, not hang until the lock
// is free.
func TestQueryLock_CancellationWhileWaitingBehindAHeldLock(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	external := flock.New(filepath.Join(txnDir, "lock"))
	locked, err := external.TryLock()
	if err != nil || !locked {
		t.Fatalf("unexpected error/result taking the external lock: locked=%v err=%v", locked, err)
	}
	defer func() { _ = external.Unlock() }()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	q := dtqlQuery("q1", "", "text")
	start := time.Now()
	_, err = store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected an error since the lock was never released")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("expected PutQuery to return promptly on cancellation, took %v", elapsed)
	}
}

// TestLoadSaveQueriesTree_NoRecursiveLockDeadlock is a regression test for
// the plan's "internal recursion must never reacquire the same
// non-reentrant lock": gofrs/flock's platform locks conflict with a second
// acquisition attempt from the same process (a different *Flock/fd on the
// same lock file), so a bug that made the recursive tree walker call the
// public, lock-acquiring LoadQueries/CreateQuery instead of their *Locked
// internals would hang here until ctx's timeout fires - which this test
// would then report as a failure, not silently pass.
func TestLoadSaveQueriesTree_NoRecursiveLockDeadlock(t *testing.T) {
	store, _ := newTestQueriesStore(t)

	tree := &datatug.QueriesFolder{
		Items: datatug.QueryDefs{
			{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "root-query", Title: "Root"}}, Type: datatug.QueryTypeSQL, Text: "SELECT 1"},
		},
		Folders: datatug.QueryFolders{
			{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "a"}},
				Items: datatug.QueryDefs{
					{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "a-query", Title: "A"}}, Type: datatug.QueryTypeSQL, Text: "SELECT 1"},
				},
				Folders: datatug.QueryFolders{
					{
						ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b"}},
						Items: datatug.QueryDefs{
							{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b-query", Title: "B"}}, Type: datatug.QueryTypeDTQL, Text: "from:\n  name: Invoice\n"},
						},
					},
				},
			},
		},
	}

	saveCtx, saveCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer saveCancel()
	if err := store.saveQueriesTree(saveCtx, "", tree); err != nil {
		t.Fatalf("unexpected error (possible recursive-lock deadlock): %v", err)
	}
	if err := saveCtx.Err(); err != nil {
		t.Fatalf("save timed out (recursive-lock deadlock): %v", err)
	}

	loadCtx, loadCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer loadCancel()
	loaded, err := store.loadQueriesTree(loadCtx, "")
	if err != nil {
		t.Fatalf("unexpected error (possible recursive-lock deadlock): %v", err)
	}
	if err := loadCtx.Err(); err != nil {
		t.Fatalf("load timed out (recursive-lock deadlock): %v", err)
	}
	if loaded == nil || len(loaded.Items) != 1 || len(loaded.Folders) != 1 {
		t.Fatalf("expected the tree to round-trip, got: %+v", loaded)
	}
}

// TestPutQuery_ConcurrentStaleWriterLosesToOneWinner races several
// goroutines each trying to update the same query from the same starting
// revision with IfMatch; exactly one may win (the store must serialize
// them, not silently accept two "matching" writers), and every loser must
// get a QueryRevisionConflictError, never a corrupted/mixed result.
func TestPutQuery_ConcurrentStaleWriterLosesToOneWinner(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "original")
	stored, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	const attempts = 8
	results := make(chan error, attempts)
	for i := 0; i < attempts; i++ {
		i := i
		go func() {
			updated := dtqlQuery("q1", "", "updated-by-writer")
			_, err := store.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: stored.Revision})
			_ = i
			results <- err
		}()
	}

	var successes, conflicts int
	for i := 0; i < attempts; i++ {
		err := <-results
		switch {
		case err == nil:
			successes++
		case datatug.IsQueryRevisionConflict(err):
			conflicts++
		default:
			t.Fatalf("unexpected error type: %T: %v", err, err)
		}
	}
	if successes != 1 {
		t.Fatalf("expected exactly one writer to win a race from the same starting revision, got %d successes", successes)
	}
	if conflicts != attempts-1 {
		t.Fatalf("expected every other writer to get a revision conflict, got %d conflicts", conflicts)
	}

	final, err := store.LoadQueryRevision(ctx, "q1")
	if err != nil {
		t.Fatalf("unexpected error loading final state: %v", err)
	}
	if final.Query.Text != "updated-by-writer" {
		t.Fatalf("expected the single winner's content, got %q", final.Query.Text)
	}
}

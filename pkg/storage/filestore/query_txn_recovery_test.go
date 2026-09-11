package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// These tests simulate a process crash by hand-assembling the exact
// on-disk artifacts a real crash would leave at a given phase (staged
// files + a committed journal, or a partially-installed pair), then
// opening a *fresh* store instance against that project directory and
// proving it recovers to a single complete revision - "old or new, never
// mixed" - before returning any result, matching the plan's Epilogue A.

func writeTestJournal(t *testing.T, txnDir string, j queryTxnJournal) {
	t.Helper()
	b, err := json.Marshal(j)
	if err != nil {
		t.Fatalf("unexpected error marshalling journal: %v", err)
	}
	if err := os.WriteFile(filepath.Join(txnDir, queryTxnJournalFile), b, 0o600); err != nil {
		t.Fatalf("unexpected error writing journal: %v", err)
	}
}

func TestRecovery_JournalWithStagedFilesCompletesToNewRevision(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonBytes := []byte(`{"id":"q1","title":"Q1","type":"DTQL"}` + "\n")
	bodyBytes := []byte("from:\n  name: Invoice\n")
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		t.Fatalf("unexpected error staging json: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		t.Fatalf("unexpected error staging body: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "", ID: "q1", Operation: queryTxnOpPut,
		JSONFileName: "q1.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "q1.query.dtql", BodyHash: hashBytes(bodyBytes),
	})

	// A fresh store instance, as if a brand new process opened the project.
	fresh := newFsQueriesStore(filepath.Dir(queriesDir))
	loaded, err := fresh.LoadQueryRevision(context.Background(), "q1")
	if err != nil {
		t.Fatalf("unexpected error recovering: %v", err)
	}
	if loaded.Query.Text != "from:\n  name: Invoice\n" {
		t.Fatalf("expected the staged body to be installed, got %q", loaded.Query.Text)
	}
	if fileExistsAt(filepath.Join(txnDir, queryTxnJournalFile)) {
		t.Error("expected the journal to be removed after recovery")
	}
	if fileExistsAt(filepath.Join(txnDir, queryTxnStagedJSON)) || fileExistsAt(filepath.Join(txnDir, queryTxnStagedBody)) {
		t.Error("expected staged files to be removed after recovery")
	}
}

func TestRecovery_PartiallyInstalledBodyCompletesJSONInstall(t *testing.T) {
	// Simulate a crash after the body was renamed into place but before the
	// JSON metadata was.
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.MkdirAll(queriesDir, 0o777); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonBytes := []byte(`{"id":"q1","title":"Q1","type":"DTQL"}` + "\n")
	bodyBytes := []byte("body content")
	// Body already installed (as if a previous run's rename succeeded)...
	if err := os.WriteFile(filepath.Join(queriesDir, "q1.query.dtql"), bodyBytes, 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// ...but JSON is still only staged.
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "", ID: "q1", Operation: queryTxnOpPut,
		JSONFileName: "q1.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "q1.query.dtql", BodyHash: hashBytes(bodyBytes),
	})

	fresh := newFsQueriesStore(filepath.Dir(queriesDir))
	loaded, err := fresh.LoadQueryRevision(context.Background(), "q1")
	if err != nil {
		t.Fatalf("unexpected error recovering: %v", err)
	}
	if loaded.Query.Text != "body content" {
		t.Fatalf("expected the already-installed body to survive recovery, got %q", loaded.Query.Text)
	}
	if !fileExistsAt(filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected the JSON metadata to be installed by recovery")
	}
}

func TestRecovery_DeleteInProgressCompletesRemoval(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q1", "", "text")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}

	// Simulate a crash right after a delete's journal was committed but
	// before either file was removed.
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "", ID: "q1", Operation: queryTxnOpDelete,
		JSONFileName: "q1.query.json", BodyFileName: "q1.query.dtql",
	})

	fresh := newFsQueriesStore(filepath.Dir(queriesDir))
	_, err = fresh.LoadQueryRevision(ctx, "q1")
	if err == nil {
		t.Fatal("expected the query to be gone after recovery completes the delete")
	}
	if fileExistsAt(filepath.Join(queriesDir, "q1.query.json")) || fileExistsAt(filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected both files to be removed by recovery")
	}
}

func TestRecovery_UnrelatedJournalDoesNotBlockADifferentQuery(t *testing.T) {
	// The leftover journal recovery finds belongs to whatever operation was
	// last in flight - not necessarily the one the current caller is
	// making. Recovery must complete it transparently before the current
	// request proceeds against its own, unrelated query.
	store, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonBytes := []byte(`{"id":"leftover","title":"Leftover","type":"DTQL"}` + "\n")
	bodyBytes := []byte("leftover body")
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "", ID: "leftover", Operation: queryTxnOpPut,
		JSONFileName: "leftover.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "leftover.query.dtql", BodyHash: hashBytes(bodyBytes),
	})

	q := dtqlQuery("q2", "", "brand new")
	stored, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if stored.Revision == "" {
		t.Fatal("expected q2 to be created despite the unrelated leftover journal")
	}
	if !fileExistsAt(filepath.Join(queriesDir, "leftover.query.json")) {
		t.Error("expected the unrelated leftover transaction to also be completed as a side effect")
	}
}

func TestRecovery_NestedFolderPathIsRederivedFromJournal(t *testing.T) {
	_, queriesDir := newTestQueriesStore(t)
	txnDir, err := ensureQueryTxnDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	jsonBytes := []byte(`{"id":"q1","title":"Q1","type":"DTQL"}` + "\n")
	bodyBytes := []byte("nested body")
	if err := writeStagedFile(txnDir, queryTxnStagedJSON, jsonBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := writeStagedFile(txnDir, queryTxnStagedBody, bodyBytes); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	writeTestJournal(t, txnDir, queryTxnJournal{
		FolderPath: "folder1/sub2", ID: "q1", Operation: queryTxnOpPut,
		JSONFileName: "q1.query.json", JSONHash: hashBytes(jsonBytes),
		BodyFileName: "q1.query.dtql", BodyHash: hashBytes(bodyBytes),
	})

	fresh := newFsQueriesStore(filepath.Dir(queriesDir))
	loaded, err := fresh.LoadQueryRevision(context.Background(), "folder1/sub2/q1")
	if err != nil {
		t.Fatalf("unexpected error recovering: %v", err)
	}
	if loaded.Query.Text != "nested body" {
		t.Fatalf("expected the nested query to be recovered, got %q", loaded.Query.Text)
	}
	if !fileExistsAt(filepath.Join(queriesDir, "folder1", "sub2", "q1.query.json")) {
		t.Error("expected the nested folder path to be re-derived and used to install the pair")
	}
}

func fileExistsAt(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

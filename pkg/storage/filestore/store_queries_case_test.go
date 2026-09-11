package filestore

import (
	"context"
	"os"
	"runtime"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// TestPutQuery_CaseInsensitiveFilesystemCollidesOnDifferentlyCasedID is N4:
// on the default case-insensitive-but-case-preserving macOS filesystem
// (APFS/HFS+), two IDs that differ only by case ("CaseTest" vs "casetest")
// address the exact same underlying file. IfNoneMatch must correctly treat
// the second create as a collision with the first, not silently duplicate
// it - a duplication bug here could not be caught anywhere else, since
// Linux CI's ext4 is case-sensitive and would never exercise this path,
// and Windows' NTFS (also case-insensitive by default) is not exercised by
// this suite at all. Skipped everywhere but macOS so a regression here
// cannot go undetected by CI (the only platform that runs it), while never
// producing a false failure on a case-sensitive filesystem where the two
// IDs are legitimately independent.
func TestPutQuery_CaseInsensitiveFilesystemCollidesOnDifferentlyCasedID(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("exercises the default case-insensitive-but-case-preserving macOS filesystem only")
	}
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()

	lower := dtqlQuery("CaseTest", "", "lower")
	stored, err := store.PutQuery(ctx, &lower, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating %q: %v", lower.ID, err)
	}

	upper := dtqlQuery("casetest", "", "upper")
	_, err = store.PutQuery(ctx, &upper, datatug.QueryWriteCondition{IfNoneMatch: true})
	if !datatug.IsQueryRevisionConflict(err) {
		t.Fatalf("expected a differently-cased id to collide with the existing record (case-insensitive filesystem), got %T: %v", err, err)
	}

	entries, err := os.ReadDir(queriesDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonFiles := 0
	for _, e := range entries {
		if !e.IsDir() {
			jsonFiles++
		}
	}
	if jsonFiles != 2 { // "CaseTest.query.json" + "CaseTest.query.dtql"
		t.Fatalf("expected exactly one query's pair of files on disk, got %d entries: %v", jsonFiles, entries)
	}

	// Load back via the original casing - the rejected write must never
	// have touched the file, so this is still the exact original record.
	loaded, err := store.LoadQueryRevision(ctx, "CaseTest")
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if loaded.Revision != stored.Revision || loaded.Query.Text != "lower" {
		t.Fatalf("expected the original record (never overwritten) to be the one on disk, got: %+v", loaded)
	}
}

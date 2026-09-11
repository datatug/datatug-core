package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// requireFileNameAliases skips t unless the file system under dir resolves
// name and alias to the same file: true for letter-case variants on the
// default case-insensitive-but-case-preserving macOS (APFS/HFS+) and
// Windows (NTFS) volumes, and for Unicode NFC/NFD variants on APFS/HFS+.
// It probes the actual volume rather than checking runtime.GOOS, so a test
// using it never falsely fails on a case-sensitive macOS volume and still
// runs wherever the aliasing it depends on really exists. Linux CI's
// case-sensitive ext4 never aliases, which is why every property these
// tests prove also has a platform-independent test (see
// TestLoadQueryRevision_RevisionIgnoresTheRequestedIDSpelling).
func requireFileNameAliases(t *testing.T, dir, name, alias string) {
	t.Helper()
	probe := filepath.Join(dir, name+".alias-probe")
	if err := os.WriteFile(probe, nil, 0o600); err != nil {
		t.Fatalf("failed to create the file-name alias probe: %v", err)
	}
	defer func() { _ = os.Remove(probe) }()
	if _, err := os.Stat(filepath.Join(dir, alias+".alias-probe")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			t.Skipf("the file system under %s does not resolve %q and %q to the same file", dir, name, alias)
		}
		t.Fatalf("failed to probe for a file-name alias: %v", err)
	}
}

// TestPutQuery_CaseInsensitiveFilesystemCollidesOnDifferentlyCasedID is N4:
// on a case-insensitive-but-case-preserving file system (the macOS and
// Windows defaults), two IDs that differ only by case ("CaseTest" vs
// "casetest") address the exact same underlying file. IfNoneMatch must
// correctly treat the second create as a collision with the first, not
// silently duplicate it. Linux CI's ext4 is case-sensitive and would never
// exercise this path, where the two IDs are legitimately independent, so
// the test is skipped wherever the volume under test does not alias them
// (requireFileNameAliases).
func TestPutQuery_CaseInsensitiveFilesystemCollidesOnDifferentlyCasedID(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	requireFileNameAliases(t, filepath.Dir(queriesDir), "CaseTest", "casetest")
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
	// (Loading via the other casing yields the same revision too - see
	// TestLoadQueryRevision_FileNameAliasesOfOneRecordShareItsRevision.)
	loaded, err := store.LoadQueryRevision(ctx, "CaseTest")
	if err != nil {
		t.Fatalf("unexpected error loading: %v", err)
	}
	if loaded.Revision != stored.Revision || loaded.Query.Text != "lower" {
		t.Fatalf("expected the original record (never overwritten) to be the one on disk, got: %+v", loaded)
	}
}

// TestLoadQueryRevision_FileNameAliasesOfOneRecordShareItsRevision is the
// regression test for a revision that depended on how the caller spelled
// the id: on a file system that resolves two spellings of one name to the
// same file, loading a record through either spelling reads exactly the
// same bytes, so it must yield exactly the revision PutQuery returned.
// Otherwise a caller that reloads via another spelling - a case-insensitive
// ID scheme, or plain caller inconsistency - is refused with a spurious
// QueryRevisionConflictError on its next conditional write or delete, so
// the test drives both of those through the alias as well. The Unicode
// normalization case is why the fix is to hash nothing the id's spelling
// influences, rather than to case-fold it.
func TestLoadQueryRevision_FileNameAliasesOfOneRecordShareItsRevision(t *testing.T) {
	for _, tt := range []struct{ name, id, alias string }{
		{name: "letter case", id: "CaseTest", alias: "casetest"},
		{name: "unicode normalization", id: "Caf\u00e9", alias: "Cafe\u0301"}, // NFC vs NFD
	} {
		t.Run(tt.name, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			requireFileNameAliases(t, filepath.Dir(queriesDir), tt.id, tt.alias)
			ctx := context.Background()

			created := dtqlQuery(tt.id, "", "original")
			stored, err := store.PutQuery(ctx, &created, datatug.QueryWriteCondition{IfNoneMatch: true})
			if err != nil {
				t.Fatalf("unexpected error creating %q: %v", tt.id, err)
			}
			for _, id := range []string{tt.id, tt.alias} {
				loaded, err := store.LoadQueryRevision(ctx, id)
				if err != nil {
					t.Fatalf("unexpected error loading %q via %q: %v", tt.id, id, err)
				}
				if loaded.Revision != stored.Revision || loaded.Query.Text != "original" {
					t.Fatalf("loading %q via %q: expected revision %s and the original text, got revision %s and %q",
						tt.id, id, stored.Revision, loaded.Revision, loaded.Query.Text)
				}
			}

			updated := dtqlQuery(tt.alias, "", "updated")
			written, err := store.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: stored.Revision})
			if err != nil {
				t.Fatalf("expected a conditional write via %q with the revision of %q to succeed, got %T: %v", tt.alias, tt.id, err, err)
			}
			reloaded, err := store.LoadQueryRevision(ctx, tt.id)
			if err != nil {
				t.Fatalf("unexpected error reloading %q: %v", tt.id, err)
			}
			if reloaded.Revision != written.Revision || reloaded.Query.Text != "updated" {
				t.Fatalf("expected %q to hold the write made via %q (revision %s), got revision %s and %q",
					tt.id, tt.alias, written.Revision, reloaded.Revision, reloaded.Query.Text)
			}

			if err := store.DeleteQueryRevision(ctx, tt.id, written.Revision); err != nil {
				t.Fatalf("expected a conditional delete via %q with the revision written via %q to succeed, got %T: %v", tt.id, tt.alias, err, err)
			}
			if _, err := store.LoadQueryRevision(ctx, tt.alias); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("expected the record to be gone via %q too, got %v", tt.alias, err)
			}
		})
	}
}

// TestLoadQueryRevision_RevisionIgnoresTheRequestedIDSpelling is the
// platform-independent counterpart of
// TestLoadQueryRevision_FileNameAliasesOfOneRecordShareItsRevision, so the
// property is guarded on Linux CI's case-sensitive ext4 as well: hard links
// give one physical pair a second name, exactly as a case-insensitive file
// system gives a record a differently-cased one. The revision must depend
// only on the bytes read, never on the spelling of the id used to find
// them.
func TestLoadQueryRevision_RevisionIgnoresTheRequestedIDSpelling(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()

	created := dtqlQuery("original", "", "SELECT 1")
	stored, err := store.PutQuery(ctx, &created, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatalf("unexpected error creating: %v", err)
	}
	for _, fileName := range []func(id string) string{
		func(id string) string { return storage.JsonFileName(id, storage.QueryFileSuffix) },
		func(id string) string { return queryBodyFileName(id, datatug.QueryTypeDTQL) },
	} {
		if err := os.Link(filepath.Join(queriesDir, fileName("original")), filepath.Join(queriesDir, fileName("alias"))); err != nil {
			// Skip only when hard links are genuinely unavailable; anything
			// else (e.g. the pair's file naming changed and the source no
			// longer exists) must fail, not silently skip.
			if errors.Is(err, errors.ErrUnsupported) || errors.Is(err, os.ErrPermission) {
				t.Skipf("the file system does not support hard links: %v", err)
			}
			t.Fatalf("failed to hard-link %s: %v", fileName("original"), err)
		}
	}

	loaded, err := store.LoadQueryRevision(ctx, "alias")
	if err != nil {
		t.Fatalf("unexpected error loading via the alias: %v", err)
	}
	if loaded.Revision != stored.Revision {
		t.Fatalf("expected the same physical pair to have the same revision via any name, got %s via the alias vs %s", loaded.Revision, stored.Revision)
	}
}

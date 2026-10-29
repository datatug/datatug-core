package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDeclaredQueryFolderFailuresDiscardPartialCatalog(t *testing.T) {
	originalOpen, originalStat := openQueryFolderDirectory, dirLstat
	t.Cleanup(func() { openQueryFolderDirectory = originalOpen; dirLstat = originalStat })
	disk := errors.New("folder IO")
	for _, mode := range []string{"missing root", "stat", "open", "read", "close", "cancel", "cancel walk", "bad declaration", "bad child path", "stuck child"} {
		t.Run(mode, func(t *testing.T) {
			openQueryFolderDirectory, dirLstat = originalOpen, originalStat
			store, root := newTestQueriesStore(t)
			_ = os.MkdirAll(filepath.Join(root, "reports"), 0700)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			guard := queryLockGuard{}
			switch mode {
			case "missing root":
				_ = os.RemoveAll(root)
			case "cancel walk":
				cancel()
			case "stat":
				dirLstat = func(p string) (os.FileInfo, error) {
					if p == root {
						return nil, disk
					}
					return originalStat(p)
				}
			case "bad declaration":
				_ = os.WriteFile(filepath.Join(root, "reports", queryFolderDeclarationFile), []byte(`{"version":99,"name":"reports"}`), 0600)
			case "bad child path":
				openQueryFolderDirectory = func(p string) (connectionDirectoryReader, error) {
					f, err := originalOpen(p)
					if err != nil {
						return nil, err
					}
					return connectionDirectoryFault{connectionDirectoryReader: f, afterRead: func() {
						_ = os.RemoveAll(filepath.Join(root, "reports"))
						_ = os.Symlink(t.TempDir(), filepath.Join(root, "reports"))
					}}, nil
				}
			case "stuck child":
				guard.stuck = []stuckQueryTxn{{}}
			default:
				openQueryFolderDirectory = func(p string) (connectionDirectoryReader, error) {
					if mode == "open" {
						return nil, disk
					}
					f, err := originalOpen(p)
					if err != nil {
						return nil, err
					}
					fault := connectionDirectoryFault{connectionDirectoryReader: f}
					switch mode {
					case "read":
						fault.readErr = disk
					case "close":
						fault.closeErr = disk
					case "cancel":
						cancel()
					}
					return fault, nil
				}
			}
			out, err := store.declaredQueryFoldersLocked(ctx, guard, store.newListingBudget(), 1000, 10000)
			if mode == "missing root" {
				if err != nil || out == nil || len(out) != 0 {
					t.Fatal("missing root must be an empty declaration catalog", out, err)
				}
				return
			}
			if err == nil || out != nil {
				t.Fatal("failed traversal exposed partial folders", out, err)
			}
		})
	}
}

func TestQueryFolderAdmissionMetadataFailureAndDirectoryReadBudget(t *testing.T) {
	store, root := newTestQueriesStore(t)
	dir := filepath.Join(root, "reports")
	_ = os.MkdirAll(dir, 0700)
	if err := writeQueryFolderDeclaration(dir, "reports"); err != nil {
		t.Fatal(err)
	}
	if err := writeQueryFolderDeclaration(dir, "reports"); err != nil {
		t.Fatal("existing valid marker should be idempotent", err)
	}
	_ = os.WriteFile(filepath.Join(dir, queryFolderDeclarationFile), []byte(`broken`), 0600)
	if err := writeQueryFolderDeclaration(dir, "reports"); err == nil {
		t.Fatal("invalid existing metadata overwritten")
	}
	if tree, err := store.loadQueriesTree(context.Background(), "reports"); err == nil || tree != nil {
		t.Fatal("tree skipped corrupt empty folder declaration")
	}
	original := dirLstat
	t.Cleanup(func() { dirLstat = original })
	failure := errors.New("stat failed")
	for _, suffix := range []string{"new-folder", "new-folder/README.md"} {
		dirLstat = func(p string) (os.FileInfo, error) {
			if p == filepath.Join(root, filepath.FromSlash(suffix)) {
				return nil, failure
			}
			return original(p)
		}
		if _, err := store.admitQueryFolderDeclaration(context.Background(), queryLockGuard{}, "", "new-folder"); !errors.Is(err, failure) {
			t.Fatal("metadata stat failure ignored", suffix, err)
		}
	}
	dirLstat = original
	// Reserving bytes must happen for both declaration and full-tree readers.
	store.listingBudget = 100
	if _, err := store.admitQueryFolderDeclaration(context.Background(), queryLockGuard{}, "", "new-folder"); err == nil {
		t.Fatal("metadata reservation ignored")
	}
	store.listingBudget = maxQueryListingBytes
	remaining := 100
	if out, err := readQueryFolderEntries(context.Background(), root, &queryReadBudget{remaining: 0}, &remaining); err == nil || out != nil {
		t.Fatal("entry-name budget failure exposed partial entries")
	}
}

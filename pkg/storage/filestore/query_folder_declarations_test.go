package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestQueryFolderDeclarationsRetainEmptyFoldersWithoutListingUnrelatedDirectories(t *testing.T) {
	s, root := newTestQueriesStore(t)
	ctx := context.Background()
	if err := s.CreateQueryFolder(ctx, "", "reports"); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateQueryFolder(ctx, "reports", "monthly"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "unrelated"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated", "README.md"), []byte("not a declaration"), 0o644); err != nil {
		t.Fatal(err)
	}
	paths, err := s.DeclaredQueryFolders(ctx)
	if err != nil || !reflect.DeepEqual(paths, []string{"reports", "reports/monthly"}) {
		t.Fatalf("declarations=%v error=%v", paths, err)
	}
	tree, err := s.loadQueriesTree(ctx, "")
	if err != nil || len(tree.Folders) != 1 || tree.Folders[0].ID != "reports" || len(tree.Folders[0].Folders) != 1 || tree.Folders[0].Folders[0].ID != "monthly" {
		t.Fatalf("empty declarations missing on reload: %v", err)
	}
	if err = s.CreateQueryFolder(ctx, "", "reports"); err != nil {
		t.Fatal("idempotent create failed", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	if out, err := s.DeclaredQueryFolders(ctx); out != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled declaration scan returned data")
	}
}

func TestQueryFolderRepeatCreateValidatesTheCompleteTarget(t *testing.T) {
	for _, mode := range []string{"valid repeat", "outside symlink", "inside symlink", "file target", "malformed declaration", "target alias", "interrupted target"} {
		t.Run(mode, func(t *testing.T) {
			store, root := newTestQueriesStore(t)
			ctx := context.Background()
			if err := store.CreateQueryFolder(ctx, "", "reports"); err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(root, "reports")
			outside := t.TempDir()
			name := "reports"
			physicalTarget := directory
			switch mode {
			case "outside symlink", "inside symlink":
				destination := outside
				if mode == "inside symlink" {
					destination = filepath.Join(root, "original")
					if err := os.Mkdir(destination, 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.RemoveAll(directory); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(destination, queryFolderDeclarationFile), []byte(`{"version":1,"name":"reports"}`), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(destination, directory); err != nil {
					t.Fatal(err)
				}
				physicalTarget = destination
			case "file target":
				if err := os.RemoveAll(directory); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(directory, []byte("retained file"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "malformed declaration":
				if err := os.WriteFile(filepath.Join(directory, queryFolderDeclarationFile), []byte(`{"version":2,"name":"reports"}`), 0o644); err != nil {
					t.Fatal(err)
				}
			case "target alias":
				name = "reports/."
			}
			before, outsideBefore := snapshotDir(t, root), snapshotDir(t, outside)
			var contentBefore map[string]string
			if mode != "file target" {
				contentBefore = snapshotDir(t, physicalTarget)
			}
			var err error
			if mode == "interrupted target" {
				_, err = store.admitQueryFolderDeclaration(ctx, queryLockGuard{stuck: []stuckQueryTxn{{err: &queryTxnIncompleteError{Err: errors.New("interrupted write")}}}}, "", name)
			} else {
				err = store.CreateQueryFolder(ctx, "", name)
			}
			if mode == "valid repeat" && err != nil || mode != "valid repeat" && err == nil {
				t.Fatalf("repeat create %s returned %v", mode, err)
			}
			if !reflect.DeepEqual(before, snapshotDir(t, root)) || !reflect.DeepEqual(outsideBefore, snapshotDir(t, outside)) {
				t.Fatal("repeat create changed the target or outside files")
			}
			if contentBefore != nil && !reflect.DeepEqual(contentBefore, snapshotDir(t, physicalTarget)) {
				t.Fatal("repeat create changed retained folder contents")
			}
		})
	}
}

func TestQueryFolderCreateRechecksTargetChangedAfterAdmission(t *testing.T) {
	store, root := newTestQueriesStore(t)
	ctx := context.Background()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, queryFolderDeclarationFile), []byte(`{"version":1,"name":"reports"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.withQueryLock(ctx, func(guard queryLockGuard) error {
		already, err := store.admitQueryFolderDeclaration(ctx, guard, "", "reports")
		if err != nil || already {
			t.Fatalf("missing folder was not admitted: %v %v", already, err)
		}
		// A filesystem writer outside the advisory lock can replace a target
		// after admission. The physical create must perform its own safe walk.
		if err := os.Symlink(outside, filepath.Join(root, "reports")); err != nil {
			return err
		}
		before, outsideBefore := snapshotDir(t, root), snapshotDir(t, outside)
		if err := store.createQueryFolderLocked("", "reports"); err == nil {
			t.Fatal("physical create followed a target swapped after admission")
		}
		if !reflect.DeepEqual(before, snapshotDir(t, root)) || !reflect.DeepEqual(outsideBefore, snapshotDir(t, outside)) {
			t.Fatal("failed physical create changed the target or outside contents")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestQueryFolderCreationCapacityDoesNotPoisonTheNextListing(t *testing.T) {
	for _, limit := range []string{"depth", "directories", "entries", "bytes"} {
		t.Run(limit, func(t *testing.T) {
			s, root := newTestQueriesStore(t)
			ctx := context.Background()
			parent := ""
			switch limit {
			case "depth":
				parent = strings.TrimSuffix(strings.Repeat("nested/", 32), "/")
				if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(parent)), 0o755); err != nil {
					t.Fatal(err)
				}
			case "directories":
				for i := 0; i < 998; i++ {
					if err := os.MkdirAll(filepath.Join(root, fmt.Sprintf("existing-%04d", i)), 0o755); err != nil {
						t.Fatal(err)
					}
				}
				if err := s.CreateQueryFolder(ctx, "", "last-admitted"); err != nil {
					t.Fatal(err)
				}
			case "entries":
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				for i := 0; i < 9997; i++ {
					if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("existing-%05d", i)), nil, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			case "bytes":
				s.listingBudget = 300
			}
			name := "refused"
			if limit == "bytes" {
				// Reach the real byte boundary without changing the reader's limit.
				for i := 0; i < 20; i++ {
					name = fmt.Sprintf("folder-%d", i)
					if err := s.CreateQueryFolder(ctx, "", name); err != nil {
						break
					}
					if i == 19 {
						t.Fatal("test did not reach byte capacity")
					}
				}
			} else if err := s.CreateQueryFolder(ctx, parent, name); err == nil {
				t.Fatal("creation exceeded prospective inventory capacity")
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(parent), name)); !os.IsNotExist(err) {
				t.Fatal("rejected capacity write created the directory")
			}
			if _, err := s.DeclaredQueryFolders(ctx); err != nil {
				t.Fatalf("rejected creation poisoned next listing: %v", err)
			}
		})
	}
}

func TestQueryFolderDeclarationFailurePublishesNoPartialMetadataAndCanRetry(t *testing.T) {
	original := environmentOS
	t.Cleanup(func() { environmentOS = original })
	for _, boundary := range []string{"write", "sync", "close"} {
		t.Run(boundary, func(t *testing.T) {
			s, root := newTestQueriesStore(t)
			ctx := context.Background()
			environmentOS = original
			environmentOS.CreateTemp = func(dir, pattern string) (environmentFile, error) {
				f, err := original.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				return environmentFaultFile{environmentFile: f, boundary: func(actual string) bool { return actual == boundary }}, nil
			}
			if err := s.CreateQueryFolder(ctx, "", "reports"); !errors.Is(err, errEnvironmentDisk) {
				t.Fatalf("fault not propagated: %v", err)
			}
			if _, err := os.Stat(filepath.Join(root, "reports", queryFolderDeclarationFile)); !os.IsNotExist(err) {
				t.Fatal("failed metadata write left final declaration")
			}
			if out, err := s.DeclaredQueryFolders(ctx); err != nil || len(out) != 0 {
				t.Fatalf("partial marker poisoned listing: %v", err)
			}
			environmentOS = original
			if err := s.CreateQueryFolder(ctx, "", "reports"); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			if out, err := s.DeclaredQueryFolders(ctx); err != nil || !reflect.DeepEqual(out, []string{"reports"}) {
				t.Fatalf("retry not discoverable: %v", err)
			}
		})
	}
}

func TestQueryFolderDeclarationMalformedOrOverBudgetReturnsNoPartialInventory(t *testing.T) {
	s, root := newTestQueriesStore(t)
	ctx := context.Background()
	if err := s.CreateQueryFolder(ctx, "", "reports"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{`{"version":1,"name":"different"}`, `{"version":1,"name":"wrong","name":"reports"}`, `{"version":1,"name":"reports","future":true}`} {
		if err := os.WriteFile(filepath.Join(root, "reports", queryFolderDeclarationFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, err := s.DeclaredQueryFolders(ctx); out != nil || err == nil {
			t.Fatal("malformed declaration accepted")
		}
	}
	if err := os.WriteFile(filepath.Join(root, "reports", queryFolderDeclarationFile), []byte(`{"version":1,"name":"reports"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s.listingBudget = 1
	if out, err := s.DeclaredQueryFolders(ctx); out != nil || err == nil {
		t.Fatal("over-budget declaration scan returned partial data")
	}
}

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

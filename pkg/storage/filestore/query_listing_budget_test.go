package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review SF2: the 16 MiB cap applies per file, so a listing of many capped
// files had no overall bound. One listing call - LoadQueries, or
// LoadProject's query tree - now reads at most its listingBudget
// (maxQueryListingBytes) of query files, checking each file's size before
// opening it. These tests lower the budget to a few hundred bytes.

// newBudgetTestProject creates a project with a project file and the given
// queries (id -> body), written through PutQuery, and returns its store
// with listingBudget set to budget.
func newBudgetTestProject(t *testing.T, budget int64, queries map[string]string) (fsProjectStore, string) {
	t.Helper()
	projectDir := t.TempDir()
	writePreflightProjectFile(t, projectDir)
	ps := newFsProjectStore("p", projectDir)
	for fullID, body := range queries {
		folder, id := splitQueryFullID(fullID)
		q := dtqlQuery(id, folder, body)
		if _, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
			t.Fatalf("unexpected error creating %s: %v", fullID, err)
		}
	}
	ps.fsQueriesStore.listingBudget = budget
	return ps, filepath.Join(projectDir, "queries")
}

func requireListingTooLarge(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected the listing to be refused over its byte budget, got nil", what)
	}
	if !strings.Contains(err.Error(), errQueryListingTooLarge.Error()) {
		t.Fatalf("%s: expected a listing-budget refusal, got: %v", what, err)
	}
}

func TestLoadQueries_RefusesAListingOverItsByteBudget(t *testing.T) {
	body := strings.Repeat("x", 600)
	ps, _ := newBudgetTestProject(t, 1000, map[string]string{"a": body, "b": body})
	ctx := context.Background()

	_, err := ps.LoadQueries(ctx, "")
	requireListingTooLarge(t, "LoadQueries", err)
	if !errors.Is(err, errQueryListingTooLarge) {
		t.Errorf("expected errors.Is(err, errQueryListingTooLarge), got: %v", err)
	}
	_, err = ps.LoadProject(ctx)
	requireListingTooLarge(t, "LoadProject", err)

	// Reading one query needs no budget.
	for _, id := range []string{"a", "b"} {
		if q, err := ps.LoadQuery(ctx, id); err != nil || q.Text != body {
			t.Errorf("LoadQuery(%s): expected the body, got err=%v", id, err)
		}
		if _, err := ps.LoadQueryRevision(ctx, id); err != nil {
			t.Errorf("LoadQueryRevision(%s): %v", id, err)
		}
	}

	// Within the budget, the same listing works.
	ps.fsQueriesStore.listingBudget = 1 << 20
	folder, err := ps.LoadQueries(ctx, "")
	if err != nil || len(folder.Items) != 2 {
		t.Fatalf("expected both queries within a larger budget, got %v (err %v)", folder, err)
	}
}

// A metadata file is charged too: a budget smaller than the first JSON
// file refuses the listing while loadDir reads it, and errors.Is still
// sees the refusal through loadDir's FilesLoadError.
func TestLoadQueries_ChargesMetadataFilesToTheBudget(t *testing.T) {
	ps, _ := newBudgetTestProject(t, 10, map[string]string{"a": ""})
	_, err := ps.LoadQueries(context.Background(), "")
	requireListingTooLarge(t, "LoadQueries", err)
	if !errors.Is(err, errQueryListingTooLarge) {
		t.Errorf("expected errors.Is to see the refusal through loadDir's error, got: %v", err)
	}
}

// LoadProject's query tree walks every folder under one budget, so folders
// that each fit on their own can still take the walk over it.
func TestLoadQueriesTree_SharesOneBudgetAcrossFolders(t *testing.T) {
	body := strings.Repeat("x", 600)
	ps, _ := newBudgetTestProject(t, 1000, map[string]string{"a": body, "sub/b": body})
	ctx := context.Background()
	for _, folder := range []string{"", "sub"} {
		if _, err := ps.LoadQueries(ctx, folder); err != nil {
			t.Fatalf("LoadQueries(%q): expected one folder to fit the budget, got: %v", folder, err)
		}
	}
	_, err := ps.loadQueriesTree(ctx, "")
	requireListingTooLarge(t, "loadQueriesTree", err)
}

func TestListingBudget_Defaults(t *testing.T) {
	if got := newFsQueriesStore(t.TempDir()).listingBudget; got != maxQueryListingBytes {
		t.Errorf("expected a new store's listing budget to be maxQueryListingBytes (%d), got %d", maxQueryListingBytes, got)
	}
	if b := (fsQueriesStore{}).newListingBudget(); b.limit != maxQueryListingBytes || b.remaining != maxQueryListingBytes {
		t.Errorf("expected a zero-value store to get the default budget, got %+v", b)
	}
	var unlimited *queryReadBudget
	if err := unlimited.take("f", 1<<40); err != nil {
		t.Errorf("expected a nil budget to be unlimited, got: %v", err)
	}
}

func TestReadRegularFileBudgeted_ChargesNothingWhenRefused(t *testing.T) {
	f := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(f, []byte("0123456789"), 0o600); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b := &queryReadBudget{remaining: 15, limit: 15}
	if _, _, err := readRegularFileBudgeted(f, maxQueryFileSize, b); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, _, err := readRegularFileBudgeted(f, maxQueryFileSize, b); !errors.Is(err, errQueryListingTooLarge) {
		t.Fatalf("expected the second read to pass the budget, got: %v", err)
	}
	if b.remaining != 5 {
		t.Errorf("expected a refused read to charge nothing, remaining %d", b.remaining)
	}
}

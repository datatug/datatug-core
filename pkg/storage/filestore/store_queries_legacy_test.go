package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Task 4: repair the legacy query write and delete adapters. These prove
// each bug fixed and that legacy compatibility (unconditional
// create/update, tolerant load, folder-embedded-in-id for UpdateQuery/
// DeleteQuery) is preserved.

func TestSaveQuery_HonorsFolderPath(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := tugqlQuery("q1", "customers/invoices", "from:\n  name: Invoice\n")
	if err := store.SaveQuery(context.Background(), &q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "customers", "invoices", "q1.query.json")) {
		t.Error("expected SaveQuery to write under its FolderPath")
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected SaveQuery not to also write at the queries root")
	}
}

func TestSaveQuery_IsUnconditionalUpsert(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	ctx := context.Background()
	first := tugqlQuery("q1", "", "first")
	if err := store.SaveQuery(ctx, &first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second := tugqlQuery("q1", "", "second")
	if err := store.SaveQuery(ctx, &second); err != nil {
		t.Fatalf("unexpected error overwriting: %v", err)
	}
	loaded, err := store.LoadQuery(ctx, "q1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if loaded.Text != "second" {
		t.Fatalf("expected SaveQuery to overwrite unconditionally, got %q", loaded.Text)
	}
}

func TestUpdateQuery_DoesNotSerializeTextIntoJSON(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeSQL,
		Text:        "SELECT 1",
	}
	if _, err := store.UpdateQuery(ctx, q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	jsonBytes, err := os.ReadFile(filepath.Join(queriesDir, "q1.query.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(string(jsonBytes), "SELECT 1") {
		t.Fatalf("expected the JSON sidecar not to contain the query text, got: %s", jsonBytes)
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Error("expected the body sidecar to exist")
	}
}

func TestUpdateQuery_TypeChangeRemovesStaleSidecar(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	sql := datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeSQL,
		Text:        "SELECT 1",
	}
	if _, err := store.UpdateQuery(ctx, sql); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Fatal("expected q1.query.sql to exist")
	}

	tugql := datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeTugQL,
		Text:        "from:\n  name: Invoice\n",
	}
	if _, err := store.UpdateQuery(ctx, tugql); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Error("expected the stale .sql sidecar to be removed on a type change")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.tugql")) {
		t.Error("expected the new .tugql sidecar to exist")
	}
}

func TestReadQueryTextSidecar_FallbackBetweenTugQLAndTugQL(t *testing.T) {
	dir := t.TempDir()

	// 1. TugQL query loading from legacy .query.tugql
	if err := os.WriteFile(filepath.Join(dir, "q1.query.tugql"), []byte("from:\n  name: Invoice\n"), 0600); err != nil {
		t.Fatal(err)
	}
	tugqlQuery := &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1"}},
		Type:        datatug.QueryTypeTugQL,
	}
	got, err := readQueryTextSidecar(dir, tugqlQuery, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "from:\n  name: Invoice\n" {
		t.Errorf("expected TugQL query to fall back to .query.tugql, got %q", got)
	}

	// 2. TugQL query loading from canonical .query.tugql
	if err := os.WriteFile(filepath.Join(dir, "q2.query.tugql"), []byte("from:\n  name: Order\n"), 0600); err != nil {
		t.Fatal(err)
	}
	legacyQuery := &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q2"}},
		Type:        datatug.QueryTypeTugQL,
	}
	got2, err := readQueryTextSidecar(dir, legacyQuery, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got2 != "from:\n  name: Order\n" {
		t.Errorf("expected TugQL query to fall back to .query.tugql, got %q", got2)
	}

	// 3. TugQL query with neither sidecar present
	q3 := &datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q3"}},
		Type:        datatug.QueryTypeTugQL,
	}
	got3, err := readQueryTextSidecar(dir, q3, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got3 != "" {
		t.Errorf("expected empty string when neither sidecar exists, got %q", got3)
	}
}

func TestUpdateQuery_HonorsFolderEmbeddedInID(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "folder1/q1", Title: "Q1"}},
		Type:        datatug.QueryTypeSQL,
		Text:        "SELECT 1",
	}
	if _, err := store.UpdateQuery(context.Background(), q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "folder1", "q1.query.json")) {
		t.Error("expected UpdateQuery to honor a folder embedded in the id")
	}
}

func TestDeleteQuery_RemovesBodySidecarToo(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := tugqlQuery("q1", "", "text")
	if err := store.SaveQuery(ctx, &q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.DeleteQuery(ctx, "q1"); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected q1.query.json to be removed")
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.tugql")) {
		t.Error("expected the body sidecar q1.query.tugql to also be removed")
	}
}

func TestDeleteQuery_MissingRecordIsNotAnError(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	if err := store.DeleteQuery(context.Background(), "does-not-exist"); err != nil {
		t.Fatalf("expected deleting a missing record to be a no-op, got: %v", err)
	}
}

func TestDeleteQuery_RejectsUnsafeLocation(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	err := store.DeleteQuery(context.Background(), "../escape")
	if !datatug.IsInvalidQueryLocation(err) {
		t.Fatalf("expected an InvalidQueryLocationError, got %T: %v", err, err)
	}
}

func TestCreateQuery_RoutesThroughPairTransaction(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	q := tugqlQuery("q1", "folder1", "from:\n  name: Invoice\n")
	if _, err := store.CreateQuery(context.Background(), q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "folder1", "q1.query.json")) {
		t.Error("expected q1.query.json to exist under folder1")
	}
	if !fileExists(t, filepath.Join(queriesDir, "folder1", "q1.query.tugql")) {
		t.Error("expected q1.query.tugql to exist under folder1")
	}
}

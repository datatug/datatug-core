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
	q := dtqlQuery("q1", "customers/invoices", "from:\n  name: Invoice\n")
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
	first := dtqlQuery("q1", "", "first")
	if err := store.SaveQuery(ctx, &first); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second := dtqlQuery("q1", "", "second")
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

	dtql := datatug.QueryDef{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "q1", Title: "Q1"}},
		Type:        datatug.QueryTypeDTQL,
		Text:        "from:\n  name: Invoice\n",
	}
	if _, err := store.UpdateQuery(ctx, dtql); err != nil {
		t.Fatalf("unexpected error updating: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.sql")) {
		t.Error("expected the stale .sql sidecar to be removed on a type change")
	}
	if !fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected the new .dtql sidecar to exist")
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
	q := dtqlQuery("q1", "", "text")
	if err := store.SaveQuery(ctx, &q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := store.DeleteQuery(ctx, "q1"); err != nil {
		t.Fatalf("unexpected error deleting: %v", err)
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.json")) {
		t.Error("expected q1.query.json to be removed")
	}
	if fileExists(t, filepath.Join(queriesDir, "q1.query.dtql")) {
		t.Error("expected the body sidecar q1.query.dtql to also be removed")
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
	q := dtqlQuery("q1", "folder1", "from:\n  name: Invoice\n")
	if _, err := store.CreateQuery(context.Background(), q); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fileExists(t, filepath.Join(queriesDir, "folder1", "q1.query.json")) {
		t.Error("expected q1.query.json to exist under folder1")
	}
	if !fileExists(t, filepath.Join(queriesDir, "folder1", "q1.query.dtql")) {
		t.Error("expected q1.query.dtql to exist under folder1")
	}
}

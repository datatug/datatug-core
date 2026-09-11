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

// Review SF-D: a symlink or other non-regular entry at one of a query's own
// file names is refused with the same typed error every other location
// refusal uses, so a caller (the capture endpoint) can map it with
// errors.As / errors.Is instead of matching text.

func requireTypedQueryLocation(t *testing.T, label string, err error, want string) {
	t.Helper()
	var locErr *datatug.InvalidQueryLocationError
	if !errors.As(err, &locErr) || !errors.Is(err, datatug.ErrInvalidQueryLocation) {
		t.Errorf("%s: expected a *datatug.InvalidQueryLocationError, got %T: %v", label, err, err)
		return
	}
	if !strings.Contains(err.Error(), want) {
		t.Errorf("%s: expected the error to mention %q, got: %v", label, want, err)
	}
}

func TestSymlinkAtTheNewBodyName_IsATypedLocationError(t *testing.T) {
	skipSymlinksOnWindows(t)
	ps, queriesDir := newPreflightProject(t)
	ctx := context.Background()
	if err := os.Symlink("/nonexistent/elsewhere", filepath.Join(queriesDir, "new.query.dtql")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := dtqlQuery("new", "", "NEW")
	_, err := ps.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireTypedQueryLocation(t, "PutQuery", err, "new.query.dtql is a symlink")
	requireTypedQueryLocation(t, "SaveQuery", ps.SaveQuery(ctx, &q), "new.query.dtql is a symlink")
	_, err = ps.CreateQuery(ctx, q)
	requireTypedQueryLocation(t, "CreateQuery", err, "new.query.dtql is a symlink")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

func TestDirectoryAtTheNewBodyName_IsATypedLocationError(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	if err := os.Mkdir(filepath.Join(queriesDir, "new.query.dtql"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := dtqlQuery("new", "", "NEW")
	_, err := ps.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireTypedQueryLocation(t, "PutQuery", err, "not a regular file")
	assertStoreUsableAfterRefusal(t, ps, queriesDir)
}

func TestSymlinkedExistingMetadata_IsATypedLocationError(t *testing.T) {
	skipSymlinksOnWindows(t)
	ps, queriesDir := newPreflightProject(t)
	ctx := context.Background()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"title":"o","type":"DTQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Symlink(outside, filepath.Join(queriesDir, "sj.query.json")); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := dtqlQuery("sj", "", "X")
	_, err := ps.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	requireTypedQueryLocation(t, "PutQuery", err, "sj.query.json is a symlink")
	requireTypedQueryLocation(t, "SaveQuery", ps.SaveQuery(ctx, &q), "sj.query.json is a symlink")
	_, err = ps.UpdateQuery(ctx, q.QueryDef)
	requireTypedQueryLocation(t, "UpdateQuery", err, "sj.query.json is a symlink")
	requireTypedQueryLocation(t, "DeleteQuery", ps.DeleteQuery(ctx, "sj"), "sj.query.json is a symlink")
	requireTypedQueryLocation(t, "DeleteQueryRevision", ps.DeleteQueryRevision(ctx, "sj", "any"), "sj.query.json is a symlink")
	_, err = ps.LoadQueryRevision(ctx, "sj")
	requireTypedQueryLocation(t, "LoadQueryRevision", err, "sj.query.json is a symlink")
	if b, _ := os.ReadFile(outside); string(b) != `{"title":"o","type":"DTQL"}` {
		t.Errorf("expected the symlink's target untouched, got %q", b)
	}
}

func TestDirectoryAtAnExistingBodyName_IsATypedLocationError(t *testing.T) {
	ps, queriesDir := newPreflightProject(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(queriesDir, "d.query.json"), []byte(`{"title":"d","type":"DTQL"}`), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := os.Mkdir(filepath.Join(queriesDir, "d.query.dtql"), 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	requireTypedQueryLocation(t, "DeleteQuery", ps.DeleteQuery(ctx, "d"), "d.query.dtql is not a regular file")
	q := dtqlQuery("d", "", "X")
	requireTypedQueryLocation(t, "SaveQuery", ps.SaveQuery(ctx, &q), "d.query.dtql is not a regular file")
}

// A refusal about a transaction artifact is not about the query's
// location, so it stays untyped.
func TestAsQueryLocationError_TypesOnlyTheQuerysOwnFileNames(t *testing.T) {
	staged := &nonRegularEntryError{path: "/p/queries/.dt-query-txn/staged.body", detail: "is a symlink; refusing to use it"}
	if err := asQueryLocationError("", "q", staged); datatug.IsInvalidQueryLocation(err) {
		t.Errorf("expected a staged-file refusal to stay untyped, got: %v", err)
	}
	other := &nonRegularEntryError{path: "/p/queries/qq.query.json", detail: "is a symlink; refusing to use it"}
	if err := asQueryLocationError("", "q", other); datatug.IsInvalidQueryLocation(err) {
		t.Errorf("expected another query's file name to stay untyped, got: %v", err)
	}
	own := &nonRegularEntryError{path: "/p/queries/q.query.json", detail: "is a symlink; refusing to use it"}
	requireTypedQueryLocation(t, "own name", asQueryLocationError("f", "q", own), "q.query.json is a symlink")
	if err := asQueryLocationError("", "q", nil); err != nil {
		t.Errorf("expected nil to stay nil, got: %v", err)
	}
}

package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func visibilityFixture(t *testing.T) (fsProjectStore, datatug.ProjectLifecycleScope) {
	t.Helper()
	root := t.TempDir()
	ctx := context.Background()
	if err := WithEnvironmentFiles(ctx, root, t.TempDir(), func(*EnvironmentFiles) error { return nil }); err != nil {
		t.Fatal(err)
	}
	s := newFsProjectStore("p", root)
	q := tugqlQuery("q", "", "from: Customer")
	if err := s.SaveQuery(ctx, &q); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "boards"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "boards", "dashboard.board.json"), []byte(`{"title":"Dashboard","queryId":"q"}`), 0600); err != nil {
		t.Fatal(err)
	}
	return s, datatug.ProjectLifecycleScope{ActorID: "owner", Storage: "local", Project: "p"}
}

func TestVisibilityFileArchiveReloadRestorePreservesReferencedDefinition(t *testing.T) {
	s, scope := visibilityFixture(t)
	ctx := context.Background()
	ref := datatug.ProjectObjectRef{Kind: "query", ID: "q"}
	before, err := os.ReadFile(filepath.Join(s.projectPath, "queries", "q.query.json"))
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.ReviewProjectLifecycle(ctx, scope, ref)
	if err != nil || !review.CanArchive || review.Complete || review.CanDelete {
		t.Fatalf("%+v %v", review, err)
	}
	w := datatug.ProjectLifecycleWrite{OperationID: "hide-q", ExpectedRevision: review.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}}
	ack, err := s.ChangeProjectLifecycle(ctx, scope, w)
	if err != nil {
		t.Fatal(err)
	}
	fresh := newFsProjectStore("p", s.projectPath)
	active, err := fresh.ListProjectLifecycle(ctx, scope, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range active.Items {
		if v.Ref == ref {
			t.Fatal("archived query still in active catalog")
		}
	}
	all, err := fresh.ListProjectLifecycle(ctx, scope, true)
	if err != nil || ack.Revision != all.Revision {
		t.Fatalf("receipt mismatch %+v %+v %v", ack, all, err)
	}
	loaded, err := fresh.LoadQuery(ctx, "q")
	if err != nil || loaded.Text != "from: Customer" {
		t.Fatalf("archiving broke query resolution: %+v %v", loaded, err)
	}
	list, err := fresh.LoadQueries(ctx, "", datatug.ActiveItemsOnly())
	if err != nil || len(list.Items) != 0 {
		t.Fatalf("active query list: %+v %v", list, err)
	}
	list, err = fresh.LoadQueries(ctx, "")
	if err != nil || len(list.Items) != 1 {
		t.Fatal("internal resolver list lost archived query", err)
	}
	bytes, err := os.ReadFile(filepath.Join(s.projectPath, "queries", "q.query.json"))
	if err != nil || string(bytes) != string(before) {
		t.Fatal("archive rewrote definition", err)
	}
	if replay, err := fresh.ChangeProjectLifecycle(ctx, scope, w); err != nil || replay != ack {
		t.Fatalf("retry differs: %+v %v", replay, err)
	}
	forged := w
	forged.Change.Action = "restore"
	if _, err = fresh.ChangeProjectLifecycle(ctx, scope, forged); !errors.Is(err, datatug.ErrProjectLifecycleConflict) {
		t.Fatal("operation ID rebound", err)
	}
	restore := datatug.ProjectLifecycleWrite{OperationID: "show-q", ExpectedRevision: all.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "restore"}}
	if _, err = fresh.ChangeProjectLifecycle(ctx, scope, restore); err != nil {
		t.Fatal(err)
	}
	if _, err = fresh.ChangeProjectLifecycle(ctx, scope, w); err != nil {
		t.Fatal(err)
	} // old retry must not hide again
	list, err = fresh.LoadQueries(ctx, "", datatug.ActiveItemsOnly())
	if err != nil || len(list.Items) != 1 {
		t.Fatal("old archive retry undid restore", err)
	}
}

func TestVisibilityFileScopeDeletionAndConcurrentRevision(t *testing.T) {
	s, scope := visibilityFixture(t)
	ctx := context.Background()
	ref := datatug.ProjectObjectRef{Kind: "query", ID: "q"}
	cat, err := s.ListProjectLifecycle(ctx, scope, true)
	if err != nil {
		t.Fatal(err)
	}
	w := datatug.ProjectLifecycleWrite{OperationID: "a", ExpectedRevision: cat.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "delete"}}
	if _, err = s.ChangeProjectLifecycle(ctx, scope, w); !errors.Is(err, datatug.ErrProjectUsageIncomplete) {
		t.Fatal("delete granted without graph", err)
	}
	bad := scope
	bad.Project = "other"
	if _, err = s.ListProjectLifecycle(ctx, bad, true); !errors.Is(err, datatug.ErrProjectLifecycleInvalid) {
		t.Fatal("scope escaped", err)
	}
	w.Change.Action = "archive"
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, id := range []string{"a", "b"} {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			write := w
			write.OperationID = id
			_, e := s.ChangeProjectLifecycle(ctx, scope, write)
			errs <- e
		}(id)
	}
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		if err == nil {
			success++
		} else if errors.Is(err, datatug.ErrProjectLifecycleConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("CAS success=%d conflict=%d", success, conflict)
	}
}

func TestVisibilityFileRuntimeTargetsAndUnsafeMetadata(t *testing.T) {
	s, scope := visibilityFixture(t)
	ctx := context.Background()
	ref := datatug.ProjectObjectRef{Kind: "session", ID: "private-session"}
	s.visibilityItems = func(_ context.Context, got datatug.ProjectLifecycleScope) ([]datatug.ProjectUsageItem, error) {
		if got.ActorID != scope.ActorID {
			t.Fatal("actor not passed")
		}
		return []datatug.ProjectUsageItem{{Ref: ref, Title: "Session"}}, nil
	}
	review, err := s.ReviewProjectLifecycle(ctx, scope, ref)
	if err != nil || !review.CanArchive {
		t.Fatal(err)
	}
	if _, err = s.ChangeProjectLifecycle(ctx, scope, datatug.ProjectLifecycleWrite{OperationID: "runtime", ExpectedRevision: review.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}}); err != nil {
		t.Fatal(err)
	}
	hidden, err := ProjectItemArchived(s.projectPath, ref)
	if err != nil || !hidden {
		t.Fatal("runtime metadata not persisted", err)
	}
	other := t.TempDir()
	secret := filepath.Join(other, "secret")
	if err = os.WriteFile(secret, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(filepath.Join(s.projectPath, visibilityFile)); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(secret, filepath.Join(s.projectPath, visibilityFile)); err != nil {
		t.Skip(err)
	}
	if _, err = s.ListProjectLifecycle(ctx, scope, true); err == nil {
		t.Fatal("symlinked metadata accepted")
	}
}

func TestVisibilityRuntimeCallbackCanReadProjectFiles(t *testing.T) {
	s, scope := visibilityFixture(t)
	s.visibilityItems = func(ctx context.Context, _ datatug.ProjectLifecycleScope) ([]datatug.ProjectUsageItem, error) {
		_, err := s.LoadEnvironments(ctx)
		return []datatug.ProjectUsageItem{{Ref: datatug.ProjectObjectRef{Kind: "session", ID: "owned"}, Title: "Owned session"}}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ref := datatug.ProjectObjectRef{Kind: "session", ID: "owned"}
	review, err := s.ReviewProjectLifecycle(ctx, scope, ref)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ChangeProjectLifecycle(ctx, scope, datatug.ProjectLifecycleWrite{OperationID: "reentry", ExpectedRevision: review.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ListProjectLifecycle(ctx, scope, true)
	if err != nil {
		t.Fatal(err)
	}
}

func TestProjectFileVisibilityObjectLegacyDefinitions(t *testing.T) {
	for _, path := range []string{"queries/a.sql.json", "queries/a.tugql.json", "queries/a.http.json", "queries/a.query.json"} {
		got, ok := ProjectFileVisibilityObject(path)
		if !ok || got != (datatug.ProjectObjectRef{Kind: "query", ID: "a"}) {
			t.Fatalf("%s: %+v %v", path, got, ok)
		}
	}
	if _, ok := ProjectFileVisibilityObject("queries/a.sql"); ok {
		t.Fatal("SQL body recognized as separate definition")
	}
}

func TestVisibilityUsesOwningQueryValidatorIdentity(t *testing.T) {
	s, scope := visibilityFixture(t)
	const filename = "query-validators/q-0123456789abcdef.json"
	const definition = `{"queryId":"folder/query","version":1}`
	if err := os.MkdirAll(filepath.Join(s.projectPath, "query-validators"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.projectPath, filename), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	if ref, ok := ProjectFileVisibilityObject(filename); ok {
		t.Fatal("invented filename identity", ref)
	}
	ref := datatug.ProjectObjectRef{Kind: "query-validator", ID: "folder/query"}
	s.visibilityItems = func(context.Context, datatug.ProjectLifecycleScope) ([]datatug.ProjectUsageItem, error) {
		return []datatug.ProjectUsageItem{{Ref: ref, Title: ref.ID}}, nil
	}
	ctx := context.Background()
	catalog, err := s.ListProjectLifecycle(ctx, scope, true)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, item := range catalog.Items {
		if item.Ref.Kind == "query-validator" {
			count++
			if item.Ref != ref {
				t.Fatal("phantom validator", item)
			}
		}
	}
	if count != 1 {
		t.Fatal("missing owning validator", catalog)
	}
	_, err = s.ChangeProjectLifecycle(ctx, scope, datatug.ProjectLifecycleWrite{OperationID: "validator", ExpectedRevision: catalog.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}})
	if err != nil {
		t.Fatal(err)
	}
	active, err := s.ListProjectLifecycle(ctx, scope, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range active.Items {
		if item.Ref == ref {
			t.Fatal("archived validator in active catalog")
		}
	}
	body, err := os.ReadFile(filepath.Join(s.projectPath, filename))
	if err != nil || string(body) != definition {
		t.Fatal("archive changed owning validator bytes", err)
	}
}

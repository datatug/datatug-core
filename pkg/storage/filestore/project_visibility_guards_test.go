package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/datatug/datatug-core/pkg/datatug"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeVisibilityFixture(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
func TestVisibilityMetadataRejectsCorruption(t *testing.T) {
	for _, body := range []string{`{`, `{"version":"1"}`, `{"version":2}`, `{"version":1,"version":1}`, `{"version":1,"archived":[{"kind":"invalid","id":"q"}]}`, `{"version":1,"archived":[{"kind":"query","id":"q"},{"kind":"query","id":"q"}]}`} {
		t.Run(body, func(t *testing.T) {
			root := t.TempDir()
			writeVisibilityFixture(t, root, visibilityFile, body)
			if _, err := readProjectVisibility(root); err == nil {
				t.Fatal("corrupt metadata accepted")
			}
			if _, err := ReadProjectArchivedItems(root); err == nil {
				t.Fatal("corrupt snapshot accepted")
			}
		})
	}
}
func TestVisibilityFileMapperLayouts(t *testing.T) {
	valid := map[string]datatug.ProjectObjectRef{
		"environments/dev/environment-summary.json":  {Kind: "environment", ID: "dev"},
		"environments/dev/dev.env.json":              {Kind: "environment", ID: "dev"},
		"queries/reports/.datatug-query-folder.json": {Kind: "query-folder", ID: "reports"},
		"queries/reports/sales.query.json":           {Kind: "query", ID: "reports/sales"},
		"boards/b/board.json":                        {Kind: "board", ID: "b"},
		"dbmodels/m/m.dbmodel.json":                  {Kind: "data-model", ID: "m"},
		"dbservers/s.dbserver.json":                  {Kind: "server", ID: "s"},
		"checks/check1.json":                         {Kind: "check", ID: "check1"},
	}
	for path, want := range valid {
		if got, ok := ProjectFileVisibilityObject(path); !ok || got != want {
			t.Fatalf("%s: %+v %v", path, got, ok)
		}
	}
	for _, path := range []string{"README.md", "runs/r.json", "environments/dev/run.json", "queries/.datatug-query-folder.json", "boards/other/b.board.json", "cards/blob.txt"} {
		if got, ok := ProjectFileVisibilityObject(path); ok {
			t.Fatalf("not definition %s: %+v", path, got)
		}
	}
}
func TestVisibilityCatalogGuards(t *testing.T) {
	for _, test := range []struct{ name, file, body string }{
		{"bad-json", "queries/bad.query.json", "{"},
		{"long-title", "queries/long.query.json", `{"title":"` + strings.Repeat("x", 1025) + `"}`},
		{"connection-invalid", "connections/one.json", `{"connections":[{"id":""}]}`},
		{"connection-long", "connections/one.json", `{"connections":[{"id":"x","title":"` + strings.Repeat("x", 1025) + `"}]}`},
		{"connection-bad-array", "connections/one.json", `{"connections":1}`},
		{"duplicate-title", "queries/q.sql.json", `{"title":"Different"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, _ := visibilityFixture(t)
			writeVisibilityFixture(t, s.projectPath, test.file, test.body)
			if _, err := s.visibilityCatalog(context.Background(), nil); err == nil {
				t.Fatal("unsafe definition accepted")
			}
		})
	}
	s, _ := visibilityFixture(t)
	writeVisibilityFixture(t, s.projectPath, "connections/one.json", `{"connections":[{"id":"a","name":"A"}],"bigQueryEditions":[{"id":"b"}],"bigQueryPlans":[{"id":"c"}]}`)
	writeVisibilityFixture(t, s.projectPath, "queries/empty.query.json", `{}`)
	for _, dir := range []string{".hidden", "node_modules", "other"} {
		writeVisibilityFixture(t, s.projectPath, dir+"/bad.json", "{")
	}
	cat, err := s.visibilityCatalog(context.Background(), nil)
	if err != nil || len(cat.Items) != 6 {
		t.Fatalf("%+v %v", cat, err)
	}
	for _, extra := range [][]datatug.ProjectUsageItem{
		{{Ref: datatug.ProjectObjectRef{Kind: "session", ID: "s"}, Restricted: true}},
		{{Ref: datatug.ProjectObjectRef{Kind: "session", ID: "s"}, Title: strings.Repeat("x", 1025)}},
		{{Ref: datatug.ProjectObjectRef{Kind: "query", ID: "q"}, Title: "conflicting"}},
	} {
		if _, err = s.visibilityCatalog(context.Background(), extra); err == nil {
			t.Fatal("invalid runtime accepted")
		}
	}
	extra := make([]datatug.ProjectUsageItem, 10001)
	for i := range extra {
		extra[i].Ref = datatug.ProjectObjectRef{Kind: "session", ID: fmt.Sprint(i)}
	}
	if _, err = s.visibilityCatalog(context.Background(), extra); err == nil {
		t.Fatal("runtime inventory budget ignored")
	}
}
func TestVisibilityIOFailures(t *testing.T) {
	sentinel := errors.New("IO failure")
	s, scope := visibilityFixture(t)
	originalWalk, originalRead, originalMarshal := visibilityWalk, visibilityRead, visibilityMarshal
	t.Cleanup(func() {
		visibilityWalk, visibilityRead, visibilityMarshal = originalWalk, originalRead, originalMarshal
	})
	info, err := os.Stat(filepath.Join(s.projectPath, "queries/q.query.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry := fs.FileInfoToDirEntry(info)
	for _, mode := range []string{"walk", "relative", "entries", "read", "missing", "cancel", "items"} {
		t.Run(mode, func(t *testing.T) {
			visibilityWalk = originalWalk
			visibilityRead = originalRead
			ctx := context.Background()
			switch mode {
			case "walk":
				visibilityWalk = func(_ string, fn fs.WalkDirFunc) error { return fn("", nil, sentinel) }
			case "relative":
				visibilityWalk = func(_ string, fn fs.WalkDirFunc) error { return fn("relative", entry, nil) }
			case "entries":
				visibilityWalk = func(root string, fn fs.WalkDirFunc) error {
					for i := 0; i < 20001; i++ {
						if e := fn(filepath.Join(root, "unrecognized"), entry, nil); e != nil {
							return e
						}
					}
					return nil
				}
			case "read":
				visibilityRead = func(string, int64, *queryReadBudget) ([]byte, bool, error) { return nil, false, sentinel }
			case "missing":
				visibilityRead = func(string, int64, *queryReadBudget) ([]byte, bool, error) { return nil, false, nil }
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "items":
				visibilityRead = func(string, int64, *queryReadBudget) ([]byte, bool, error) { return []byte(`{}`), true, nil }
				visibilityWalk = func(root string, fn fs.WalkDirFunc) error {
					for i := 0; i < 10001; i++ {
						if e := fn(filepath.Join(root, fmt.Sprintf("queries/q%d.query.json", i)), entry, nil); e != nil {
							return e
						}
					}
					return nil
				}
			}
			if _, e := s.visibilityCatalog(ctx, nil); e == nil {
				t.Fatal("failure swallowed")
			}
		})
	}
	visibilityWalk, visibilityRead = originalWalk, originalRead
	visibilityMarshal = func(any) ([]byte, error) { return nil, sentinel }
	if _, err = s.visibilityCatalog(context.Background(), nil); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	visibilityMarshal = originalMarshal
	bad := scope
	bad.ActorID = ""
	if _, err = s.ReviewProjectLifecycle(context.Background(), bad, datatug.ProjectObjectRef{}); err == nil {
		t.Fatal("bad review scope")
	}
	if _, err = s.ChangeProjectLifecycle(context.Background(), bad, datatug.ProjectLifecycleWrite{}); err == nil {
		t.Fatal("bad write scope")
	}
	if _, err = s.ChangeProjectLifecycle(context.Background(), scope, datatug.ProjectLifecycleWrite{}); err == nil {
		t.Fatal("invalid write")
	}
	runtime := NewProjectStoreWithVisibilityItems("p", s.projectPath, func(context.Context, datatug.ProjectLifecycleScope) ([]datatug.ProjectUsageItem, error) {
		return nil, sentinel
	}).(datatug.ProjectLifecycleStore)
	if _, err = runtime.ListProjectLifecycle(context.Background(), scope, true); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	if _, err = runtime.ReviewProjectLifecycle(context.Background(), scope, datatug.ProjectObjectRef{Kind: "query", ID: "q"}); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	write := datatug.ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: "r", Change: datatug.ProjectLifecycleChange{Object: datatug.ProjectObjectRef{Kind: "query", ID: "q"}, Action: "archive"}}
	if _, err = runtime.ChangeProjectLifecycle(context.Background(), scope, write); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	writeVisibilityFixture(t, s.projectPath, visibilityFile, "{")
	if _, err = s.ReviewProjectLifecycle(context.Background(), scope, write.Change.Object); err == nil {
		t.Fatal("corrupt review")
	}
	if _, err = s.ChangeProjectLifecycle(context.Background(), scope, write); err == nil {
		t.Fatal("corrupt write")
	}
}
func TestVisibilityWriteEncodingAndDurabilityFailures(t *testing.T) {
	original := visibilityMarshal
	t.Cleanup(func() { visibilityMarshal = original })
	for _, failAt := range []int{3, 4, 5} {
		t.Run(fmt.Sprint(failAt), func(t *testing.T) {
			s, scope := visibilityFixture(t)
			cat, err := s.ListProjectLifecycle(context.Background(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			n := 0
			sentinel := errors.New("encode")
			visibilityMarshal = func(v any) ([]byte, error) {
				n++
				if n == failAt {
					return nil, sentinel
				}
				return json.Marshal(v)
			}
			w := datatug.ProjectLifecycleWrite{OperationID: "op", ExpectedRevision: cat.Revision, Change: datatug.ProjectLifecycleChange{Object: datatug.ProjectObjectRef{Kind: "query", ID: "q"}, Action: "archive"}}
			if _, err = s.ChangeProjectLifecycle(context.Background(), scope, w); !errors.Is(err, sentinel) {
				t.Fatal(err)
			}
			visibilityMarshal = original
		})
	}
}

func TestVisibilityMutationFailureBoundaries(t *testing.T) {
	originalOS := environmentOS
	originalRead := visibilityRead
	t.Cleanup(func() { environmentOS = originalOS; visibilityRead = originalRead })
	sentinel := errors.New("durability")
	for _, mode := range []string{"receipt", "second-state", "before-hash", "commit"} {
		t.Run(mode, func(t *testing.T) {
			environmentOS = originalOS
			visibilityRead = originalRead
			s, scope := visibilityFixture(t)
			cat, err := s.ListProjectLifecycle(context.Background(), scope, true)
			if err != nil {
				t.Fatal(err)
			}
			w := datatug.ProjectLifecycleWrite{OperationID: "failure", ExpectedRevision: cat.Revision, Change: datatug.ProjectLifecycleChange{Object: datatug.ProjectObjectRef{Kind: "query", ID: "q"}, Action: "archive"}}
			switch mode {
			case "receipt":
				bindingBody, _ := json.Marshal(scope)
				opID := "visibility-" + environmentHash([]byte(environmentHash(bindingBody)+"/"+w.OperationID))
				if e := WithEnvironmentFiles(context.Background(), s.projectPath, "", func(f *EnvironmentFiles) error {
					return os.WriteFile(filepath.Join(f.namespace, environmentHash([]byte(opID))+".receipt.json"), []byte(`{}`), 0600)
				}); e != nil {
					t.Fatal(e)
				}
				environmentOS.ReadFile = func(p string) ([]byte, error) {
					if strings.HasSuffix(p, ".receipt.json") {
						return nil, sentinel
					}
					return originalOS.ReadFile(p)
				}
			case "second-state":
				visibilityRead = func(p string, n int64, b *queryReadBudget) ([]byte, bool, error) {
					data, exists, e := originalRead(p, n, b)
					writeVisibilityFixture(t, s.projectPath, visibilityFile, "{")
					return data, exists, e
				}
			case "before-hash":
				environmentOS.ReadFile = func(p string) ([]byte, error) {
					if p == filepath.Join(s.projectPath, visibilityFile) {
						return nil, sentinel
					}
					return originalOS.ReadFile(p)
				}
			case "commit":
				environmentOS.CreateTemp = func(string, string) (environmentFile, error) { return nil, sentinel }
			}
			if _, err = s.ChangeProjectLifecycle(context.Background(), scope, w); err == nil {
				t.Fatal("failure acknowledged")
			}
			environmentOS = originalOS
			visibilityRead = originalRead
		})
	}
	s, scope := visibilityFixture(t)
	for i, ref := range []datatug.ProjectObjectRef{{Kind: "board", ID: "dashboard"}, {Kind: "query", ID: "q"}} {
		cat, err := s.ListProjectLifecycle(context.Background(), scope, true)
		if err != nil {
			t.Fatal(err)
		}
		_, err = s.ChangeProjectLifecycle(context.Background(), scope, datatug.ProjectLifecycleWrite{OperationID: fmt.Sprint(i), ExpectedRevision: cat.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	hidden, err := ReadProjectArchivedItems(s.projectPath)
	if err != nil || len(hidden) != 2 {
		t.Fatal(hidden, err)
	}
}

func TestVisibilityActiveTreeProjections(t *testing.T) {
	root := t.TempDir()
	writeVisibilityFixture(t, root, visibilityFile, `{"version":1,"archived":[{"kind":"query","id":"folder/hidden"},{"kind":"query-folder","id":"archived"},{"kind":"query","id":"q"}]}`)
	q := func(id string) *datatug.QueryDef {
		return &datatug.QueryDef{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id}}}
	}
	tree := &datatug.QueriesFolder{Items: datatug.QueryDefs{q("q"), q("kept")}, Folders: datatug.QueryFolders{{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "folder"}}, Items: datatug.QueryDefs{q("hidden"), q("visible")}}, {ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "archived"}}}}}
	if err := filterVisibleQueryTree(root, tree, ""); err != nil {
		t.Fatal(err)
	}
	if len(tree.Items) != 1 || len(tree.Folders) != 1 || len(tree.Folders[0].Items) != 1 {
		t.Fatalf("%+v", tree)
	}
	if err := filterVisibleQueryTree(root, nil, ""); err != nil {
		t.Fatal(err)
	}
	got, err := filterVisibleProjectItems(root, "query", "folder", datatug.QueryDefs{q("hidden"), q("visible")}, datatug.ActiveItemsOnly())
	if err != nil || len(got) != 1 {
		t.Fatal(got, err)
	}
	writeVisibilityFixture(t, root, visibilityFile, "{")
	if err = filterVisibleQueryTree(root, tree, ""); err == nil {
		t.Fatal("corruption swallowed")
	}
	if _, err = filterVisibleProjectItems(root, "query", "", got, datatug.ActiveItemsOnly()); err == nil {
		t.Fatal("corruption swallowed")
	}
}

func TestVisibilityFullProjectProjection(t *testing.T) {
	root := t.TempDir()
	saveSmallProject(t, root)
	raw, err := os.ReadFile(filepath.Join(root, "datatug-project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest["environmentOrder"] = []string{"local", "dev"}
	raw, err = json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeVisibilityFixture(t, root, "datatug-project.json", string(raw))
	s := newFsProjectStore("p1", root)
	writeVisibilityFixture(t, root, visibilityFile, `{"version":1,"archived":[{"kind":"environment","id":"dev"},{"kind":"query","id":"q1"}]}`)
	project, err := s.LoadProject(context.Background(), datatug.ActiveItemsOnly())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range project.Queries.Items {
		if q.ID == "q1" {
			t.Fatal("hidden query in full summary")
		}
	}
	for _, id := range project.EnvironmentOrder {
		if id == "dev" {
			t.Fatal("hidden env order")
		}
	}
	original := projectVisibilityTreeFilter
	t.Cleanup(func() { projectVisibilityTreeFilter = original })
	sentinel := errors.New("projection read failed")
	projectVisibilityTreeFilter = func(string, *datatug.QueriesFolder, string) error { return sentinel }
	if _, err = s.LoadProject(context.Background(), datatug.ActiveItemsOnly()); !errors.Is(err, sentinel) {
		t.Fatal(err)
	}
	projectVisibilityTreeFilter = original
	// Query lists propagate malformed visibility metadata as an error.
	writeVisibilityFixture(t, root, visibilityFile, "{")
	if _, err = s.LoadQueries(context.Background(), "", datatug.ActiveItemsOnly()); err == nil {
		t.Fatal("query projection corruption swallowed")
	}
}

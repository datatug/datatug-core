package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

// Review V1: every save path - PutQuery, the legacy SaveQuery, CreateQuery
// and UpdateQuery, and a project save - refuses a query whose title or text
// carries a credential, with a typed bad-record error and before anything
// reaches disk. The same query without the secret saves on every path, so
// the refusals are the screening's and not a broken harness.

const credentialProbe = "s3cr3t-value"

type querySavePath struct {
	name string
	save func(ctx context.Context, projectDir string, q datatug.QueryDef) error
}

func querySavePaths() []querySavePath {
	return []querySavePath{
		{"PutQuery", func(ctx context.Context, dir string, q datatug.QueryDef) error {
			_, err := newFsProjectStore("p", dir).PutQuery(ctx, &datatug.QueryDefWithFolderPath{QueryDef: q}, datatug.QueryWriteCondition{IfNoneMatch: true})
			return err
		}},
		{"SaveQuery", func(ctx context.Context, dir string, q datatug.QueryDef) error {
			return newFsProjectStore("p", dir).SaveQuery(ctx, &datatug.QueryDefWithFolderPath{QueryDef: q})
		}},
		{"CreateQuery", func(ctx context.Context, dir string, q datatug.QueryDef) error {
			_, err := newFsQueriesStore(dir).CreateQuery(ctx, datatug.QueryDefWithFolderPath{QueryDef: q})
			return err
		}},
		{"UpdateQuery", func(ctx context.Context, dir string, q datatug.QueryDef) error {
			_, err := newFsQueriesStore(dir).UpdateQuery(ctx, q)
			return err
		}},
		{"SaveProject", func(ctx context.Context, dir string, q datatug.QueryDef) error {
			project := &datatug.Project{Queries: &datatug.QueriesFolder{Items: datatug.QueryDefs{&q}}}
			project.ID = "p"
			project.Access = "private"
			project.Created = &datatug.ProjectCreated{At: time.Now()}
			return newFsProjectStore("p", dir).SaveProject(ctx, project)
		}},
	}
}

func TestEverySavePath_RefusesACredentialInTheTitleOrTheText(t *testing.T) {
	cases := []struct {
		name, field, title string
		typ                datatug.QueryType
		text               string
	}{
		{"title with a URL password", "title", "prod postgres://u:" + credentialProbe + "@h/db", datatug.QueryTypeSQL, "SELECT 1"},
		{"title with Password=", "title", "Server=h;Password=" + credentialProbe, datatug.QueryTypeSQL, "SELECT 1"},
		{"SQL dblink password", "text", "t", datatug.QueryTypeSQL, "SELECT * FROM dblink('host=h user=u password=" + credentialProbe + "', 'select 1') AS t(x int)"},
		{"SQL comment URL", "text", "t", datatug.QueryTypeSQL, "-- postgres://u:" + credentialProbe + "@h/db\nSELECT 1"},
		{"DTQL comment URL", "text", "t", datatug.QueryTypeDTQL, "from:\n  name: Invoice\n# postgres://u:" + credentialProbe + "@h/db"},
		{"GraphQL header", "text", "t", "GraphQL", "# Authorization: Bearer " + credentialProbe + "\n{ a }"},
		{"HTTP URL password", "text", "t", datatug.QueryTypeHTTP, "https://u:" + credentialProbe + "@h/x"},
	}
	ctx := context.Background()
	for _, c := range cases {
		for _, sp := range querySavePaths() {
			t.Run(c.name+"/"+sp.name, func(t *testing.T) {
				dir := t.TempDir()
				var q datatug.QueryDef
				q.ID, q.Title, q.Type, q.Text = "c", c.title, c.typ, c.text
				err := sp.save(ctx, dir, q)
				if err == nil {
					t.Fatal("expected the write to be refused")
				}
				if !validation.IsBadRecordError(err) || !strings.Contains(err.Error(), c.field) {
					t.Errorf("expected a bad-record error naming %q, got: %v", c.field, err)
				}
				assertNoFileHolds(t, dir, credentialProbe)
			})
		}
	}
	for _, sp := range querySavePaths() {
		t.Run("control/"+sp.name, func(t *testing.T) {
			var q datatug.QueryDef
			q.ID, q.Title, q.Type, q.Text = "c", "Users by token", datatug.QueryTypeSQL, "SELECT * FROM users WHERE token = @token"
			if err := sp.save(ctx, t.TempDir(), q); err != nil {
				t.Fatalf("expected the same query without a secret to save, got: %v", err)
			}
		})
	}
}

func assertNoFileHolds(t *testing.T, dir, needle string) {
	t.Helper()
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			if b, _ := os.ReadFile(p); strings.Contains(string(b), needle) {
				t.Errorf("%s holds the refused content", p)
			}
		}
		return nil
	})
}

// The demo project (datatug-demo-projects/demo-project-1, read only
// through a copy) loads and re-saves through every save path with no
// refusal once titles and bodies are screened.
func TestDemoProject1Queries_ResaveOnEverySavePath(t *testing.T) {
	src := demoProject1Dir(t)
	dst := filepath.Join(t.TempDir(), "demo-project-1")
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("unexpected error copying the demo project: %v", err)
	}
	ctx := context.Background()
	store := newFsProjectStore("demo-project-1", dst)
	project, err := store.LoadProject(ctx)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	var ids []string
	var walk func(folder *datatug.QueriesFolder, prefix string)
	walk = func(folder *datatug.QueriesFolder, prefix string) {
		for _, item := range folder.Items {
			ids = append(ids, prefix+item.ID)
		}
		for _, sub := range folder.Folders {
			walk(sub, prefix+sub.ID+"/")
		}
	}
	if project.Queries == nil {
		t.Fatal("expected the demo project to hold queries")
	}
	walk(project.Queries, "")
	if len(ids) < 5 {
		t.Fatalf("expected the demo project's 5 queries, got %v", ids)
	}
	for _, id := range ids {
		stored, err := store.LoadQueryRevision(ctx, id)
		if err != nil {
			t.Errorf("%s: LoadQueryRevision: %v", id, err)
			continue
		}
		if err := stored.Query.QueryDef.Validate(); err != nil {
			t.Errorf("%s: Validate: %v", id, err)
		}
		same := stored.Query
		if err := store.SaveQuery(ctx, &same); err != nil {
			t.Errorf("%s: SaveQuery: %v", id, err)
		}
		again, err := store.LoadQueryRevision(ctx, id)
		if err != nil {
			t.Errorf("%s: reload: %v", id, err)
			continue
		}
		put := again.Query
		if _, err := store.PutQuery(ctx, &put, datatug.QueryWriteCondition{IfMatch: again.Revision}); err != nil {
			t.Errorf("%s: PutQuery IfMatch: %v", id, err)
		}
		update := again.Query.QueryDef
		update.ID = id // UpdateQuery takes the combined "<folder>/<id>"
		if _, err := store.UpdateQuery(ctx, update); err != nil {
			t.Errorf("%s: UpdateQuery: %v", id, err)
		}
		create := again.Query
		if _, err := store.CreateQuery(ctx, create); err != nil {
			t.Errorf("%s: CreateQuery: %v", id, err)
		}
	}
	if err := store.SaveProject(ctx, project); err != nil {
		t.Errorf("SaveProject(loaded demo project): %v", err)
	}
	if _, err := store.LoadProject(ctx); err != nil {
		t.Errorf("LoadProject after the round trip: %v", err)
	}
}

// capturedQueryForSave is a realistic query captured from exploration: an
// e-mail author, a collection named after password resets, and a bound
// semantic parameter. Every save path must accept it as it stands.
func capturedQueryForSave() datatug.QueryDef {
	var q datatug.QueryDef
	q.ID, q.Title, q.Type, q.Text = "c", "Customer invoices", datatug.QueryTypeDTQL, "from: Invoice\n"
	q.Purpose = "Which invoices does this customer have, and did any go unpaid?"
	q.Parameters = datatug.Parameters{{ID: "CustomerId", Type: "integer", IsRequired: true}}
	q.Capture = &datatug.QueryCapture{
		Author:      "alice@example.com",
		Environment: "prod",
		Source:      "chinook@v2",
		Collection:  "password_resets",
		Bindings:    []datatug.QueryCaptureBinding{{ParameterID: "CustomerId", Origin: datatug.QueryCaptureOriginSelection}},
	}
	return q
}

// Hub AC query-pair-storage-guards-writes, for the two fields the capture
// contract added: every save path - PutQuery, the legacy SaveQuery,
// CreateQuery and UpdateQuery, and a project save - refuses a credential
// in a query's purpose or in any string of its capture provenance, with a
// typed bad-record error and before anything reaches disk; and each of
// them still saves the realistic captured query the refusals are derived
// from, so the refusals are the screening's and not a broken harness.
func TestEverySavePath_RefusesACredentialInThePurposeOrTheCapture(t *testing.T) {
	cases := []struct {
		name, field string
		mutate      func(q *datatug.QueryDef)
	}{
		{"purpose with a URL password", "purpose", func(q *datatug.QueryDef) {
			q.Purpose = "rows from postgres://u:" + credentialProbe + "@h/db"
		}},
		{"purpose with a JSON password member", "purpose", func(q *datatug.QueryDef) {
			q.Purpose = `checks whether {"password": "` + credentialProbe + `"} still works`
		}},
		{"capture author with a connection string", "author", func(q *datatug.QueryDef) {
			q.Capture.Author = "Server=h;User Id=u;Password=" + credentialProbe + ";"
		}},
		{"capture environment with password=", "environment", func(q *datatug.QueryDef) {
			q.Capture.Environment = "password=" + credentialProbe
		}},
		{"capture source with a URL password", "source", func(q *datatug.QueryDef) {
			q.Capture.Source = "postgres://u:" + credentialProbe + "@h/db"
		}},
		{"capture collection with AccountKey=", "collection", func(q *datatug.QueryDef) {
			q.Capture.Collection = "AccountKey=" + credentialProbe
		}},
		{"binding parameter id with token=", "bindings[0]", func(q *datatug.QueryDef) {
			q.Capture.Bindings[0].ParameterID = "token=" + credentialProbe
		}},
	}
	ctx := context.Background()
	for _, c := range cases {
		for _, sp := range querySavePaths() {
			t.Run(c.name+"/"+sp.name, func(t *testing.T) {
				dir := t.TempDir()
				q := capturedQueryForSave()
				c.mutate(&q)
				err := sp.save(ctx, dir, q)
				if err == nil {
					t.Fatal("expected the write to be refused")
				}
				if !validation.IsBadRecordError(err) || !strings.Contains(err.Error(), c.field) {
					t.Errorf("expected a bad-record error naming %q, got: %v", c.field, err)
				}
				assertNoFileHolds(t, dir, credentialProbe)
			})
		}
	}
	for _, sp := range querySavePaths() {
		t.Run("control/"+sp.name, func(t *testing.T) {
			if err := sp.save(ctx, t.TempDir(), capturedQueryForSave()); err != nil {
				t.Fatalf("expected the realistic captured query to save, got: %v", err)
			}
		})
	}
}

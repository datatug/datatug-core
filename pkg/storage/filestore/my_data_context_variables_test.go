package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// myDataFixture writes a stand-in for the datatug/my-data project (backstage
// spec/features/my-data-project; the real repository does not exist yet)
// holding its "My projects" and "My user record" queries verbatim, and
// returns the project directory.
func myDataFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "my-data")
	query := func(id, title, text string) *datatug.QueryDef {
		return &datatug.QueryDef{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: id, Title: title}},
			Type:        datatug.QueryTypeTugQL,
			Text:        text,
		}
	}
	project := &datatug.Project{
		ProjectItem: datatug.ProjectItem{Access: "public", ProjItemBrief: datatug.ProjItemBrief{ID: "my-data", Title: "My DataTug Data"}},
		Created:     &datatug.ProjectCreated{At: time.Now()},
		Queries: &datatug.QueriesFolder{Items: datatug.QueryDefs{
			query("my-projects", "My projects", "FROM spaces/{SneatUserPersonalSpaceID}/ext/datatug/projects\nORDER BY title"),
			query("my-user-record", "My user record", "FROM users\nWHERE __key__ = @SneatUserID"),
		}},
	}
	if err := NewProjectStore("my-data", dir).SaveProject(context.Background(), project); err != nil {
		t.Fatalf("SaveProject: %v", err)
	}
	return dir
}

// AC core-refuses-context-variables: the project validates, and running
// "My projects" is refused naming SneatUserPersonalSpaceID. The refusal is a
// pure function of the loaded query, so no connection can have been opened.
func TestMyDataFixture_ValidatesAndRefusesToRun(t *testing.T) {
	ctx := context.Background()
	store := NewProjectStore("my-data", myDataFixture(t))
	project, err := store.LoadProject(ctx)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if err = project.Validate(); err != nil {
		t.Fatalf("expected the project to validate, got: %v", err)
	}
	for id, variable := range map[string]string{"my-projects": "SneatUserPersonalSpaceID", "my-user-record": "SneatUserID"} {
		q, err := store.LoadQuery(ctx, id)
		if err != nil {
			t.Fatalf("LoadQuery(%s): %v", id, err)
		}
		err = q.RefuseContextVariables()
		if !errors.Is(err, datatug.ErrContextVariableNotBound) || !strings.Contains(err.Error(), "context variable "+variable+",") {
			t.Errorf("%s: expected a refusal naming %s, got: %v", id, variable, err)
		}
	}
}

// AC reserved-names: the same query declaring a parameter named SneatUserID
// fails project validation, naming the variable.
func TestMyDataFixture_ReservedParameterFailsValidation(t *testing.T) {
	dir := myDataFixture(t)
	file := filepath.Join(dir, storage.QueriesFolder, "my-user-record.query.json")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var q map[string]any
	if err = json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	q["parameters"] = []any{map[string]any{"id": "SneatUserID", "type": "string"}}
	if data, err = json.Marshal(q); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	project, err := NewProjectStore("my-data", dir).LoadProject(context.Background())
	if err == nil {
		err = project.Validate()
	}
	if err == nil || !strings.Contains(err.Error(), "SneatUserID is a reserved context variable name") {
		t.Fatalf("expected validation to fail naming SneatUserID, got: %v", err)
	}
}

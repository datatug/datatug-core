package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// writeQueryFiles seeds a query pair by hand: the JSON sidecar of the given
// type (createdAt included) and one file per name in bodies.
func writeQueryFiles(t *testing.T, dir, queryType string, bodies map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	meta := `{"createdAt":"2026-10-01T00:00:00Z","id":"q","title":"Existing","type":"` + queryType + `"}`
	if err := os.WriteFile(filepath.Join(dir, "q.query.json"), []byte(meta), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range bodies {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func bodyFilePresent(dir, name string) bool {
	_, err := os.Lstat(filepath.Join(dir, name))
	return err == nil
}

// The body file's name, never its content, decides the representation.
func TestQueryBodyFiles_ReadRepresentationFollowsFileName(t *testing.T) {
	const body = "from:\n  name: Invoice\n"
	cases := []struct {
		name, declared, file string
		structured           bool
	}{
		{"text", "TugQL", "q.query.tugql", false},
		{"structured", "TugQL", "q.query.tugql.yaml", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			writeQueryFiles(t, queriesDir, tc.declared, map[string]string{tc.file: body})
			ctx := context.Background()

			check := func(label string, q datatug.QueryDef) {
				t.Helper()
				if q.Type != datatug.QueryTypeTugQL || q.StructuredBody != tc.structured || q.Text != body {
					t.Errorf("%s: type=%q structured=%v text=%q", label, q.Type, q.StructuredBody, q.Text)
				}
			}
			q, err := store.LoadQuery(ctx, "q")
			if err != nil {
				t.Fatal(err)
			}
			check("LoadQuery", *q)
			folder, err := store.LoadQueries(ctx, "")
			if err != nil || len(folder.Items) != 1 {
				t.Fatalf("LoadQueries: %v %v", folder, err)
			}
			check("LoadQueries", *folder.Items[0])
			stored, err := store.LoadQueryRevision(ctx, "q")
			if err != nil {
				t.Fatal(err)
			}
			check("LoadQueryRevision", stored.Query.QueryDef)
		})
	}
}

// Both body files for one query is an error on every read path; nothing is
// guessed.
func TestQueryBodyFiles_BothFilesPresentIsAnError(t *testing.T) {
	pairs := [][2]string{
		{"q.query.tugql", "q.query.tugql.yaml"},
	}
	for _, declared := range []string{"TugQL"} {
		for _, pair := range pairs {
			t.Run(declared+" "+pair[0]+" + "+pair[1], func(t *testing.T) {
				store, queriesDir := newTestQueriesStore(t)
				writeQueryFiles(t, queriesDir, declared, map[string]string{pair[0]: "a", pair[1]: "b"})
				ctx := context.Background()
				if _, err := store.LoadQuery(ctx, "q"); err == nil {
					t.Error("LoadQuery: expected an error")
				}
				if _, err := store.LoadQueries(ctx, ""); err == nil {
					t.Error("LoadQueries: expected an error")
				}
				if _, err := store.LoadQueryRevision(ctx, "q"); err == nil {
					t.Error("LoadQueryRevision: expected an error")
				}
			})
		}
	}
}

func TestQueryBodyFiles_ReadFailureOfACandidateIsAnError(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeQueryFiles(t, queriesDir, "TugQL", nil)
	// A directory where a body file belongs is not a regular file.
	if err := os.Mkdir(filepath.Join(queriesDir, "q.query.tugql.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadQuery(context.Background(), "q"); err == nil {
		t.Fatal("expected an error")
	}
}

// A caller can switch a query between the text and the structured body; the
// previous body file goes with the old representation.
func TestQueryBodyFiles_WritesWhicheverRepresentationTheCallerHas(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := tugqlQuery("q", "", "from Invoice")
	first, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bodyFilePresent(queriesDir, "q.query.tugql") || bodyFilePresent(queriesDir, "q.query.tugql.yaml") {
		t.Fatal("text query must write q.query.tugql only")
	}
	q.Text, q.StructuredBody = "from:\n  name: Invoice\n", true
	second, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if bodyFilePresent(queriesDir, "q.query.tugql") || !bodyFilePresent(queriesDir, "q.query.tugql.yaml") {
		t.Fatal("structured query must write q.query.tugql.yaml only")
	}
	if first.Revision == second.Revision {
		t.Fatal("changing the representation must change the revision")
	}
	loaded, err := store.LoadQueryRevision(ctx, "q")
	if err != nil || loaded.Revision != second.Revision || !loaded.Query.StructuredBody {
		t.Fatalf("reload: %+v %v", loaded, err)
	}
}

func TestQueryBodyFiles_StructuredBodyIsTugQLOnly(t *testing.T) {
	q := validQueryForWrite()
	q.Type = datatug.QueryTypeSQL
	q.StructuredBody = true
	if err := q.Validate(); err == nil {
		t.Error("Validate: expected an error for a structured non-TugQL body")
	}
	if _, err := queryBodyFileName("q", datatug.QueryTypeSQL, true); err == nil {
		t.Error("queryBodyFileName: expected an error for a structured non-TugQL body")
	}
	if name, err := queryBodyFileName("q", datatug.QueryTypeTugQL, true); err != nil || name != "q.query.tugql.yaml" {
		t.Errorf("queryBodyFileName = %q, %v", name, err)
	}
	if got := queryBodyFileExt(datatug.QueryTypeTugQL, true); got != "tugql.yaml" {
		t.Errorf("queryBodyFileExt = %q", got)
	}
}

func TestCheckQueryBodyFileName_AcceptsEveryBodyForm(t *testing.T) {
	for _, name := range []string{"q.query.tugql", "q.query.tugql.yaml", "q.query.sql"} {
		if err := checkQueryBodyFileName("q", name); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, name := range []string{"q.query.sql.yaml", "q.query.tugql.yml", "r.query.tugql"} {
		if err := checkQueryBodyFileName("q", name); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

// The pre-cutover "DTQL" type is gone (TugQL cutover step 9): a sidecar that
// still declares it, with its old structured body, is not loaded as TugQL.
func TestQueryBodyFiles_PreCutoverTypeIsNotLoaded(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	writeQueryFiles(t, queriesDir, "DTQL", map[string]string{"q.query.dtql": "from:\n  name: Invoice\n"})
	q, err := store.LoadQuery(context.Background(), "q")
	if err == nil && q.Type == datatug.QueryTypeTugQL {
		t.Fatalf("LoadQuery loaded a pre-cutover query as TugQL: %+v", q)
	}
}

// Writers never produce the pre-cutover type.
func TestQueryBodyFiles_WritingTheLegacyTypeIsRefused(t *testing.T) {
	store, _ := newTestQueriesStore(t)
	q := tugqlQuery("q", "", "text")
	q.Type = datatug.QueryType("DTQL")
	if _, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err == nil {
		t.Fatal("expected an error")
	}
}

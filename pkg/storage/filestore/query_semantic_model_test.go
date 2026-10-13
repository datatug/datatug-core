package filestore

import (
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/datatug"
	"os"
	"reflect"
	"testing"
)

func TestSemanticModelRevisionedRoundTripAndCAS(t *testing.T) {
	data, err := os.ReadFile("../../datatug/testdata/query_semantic_model.json")
	if err != nil {
		t.Fatal(err)
	}
	var model datatug.QuerySemanticModel
	if err = json.Unmarshal(data, &model); err != nil {
		t.Fatal(err)
	}
	store := NewProjectStore("test", t.TempDir()).(datatug.RevisionedQueriesStore)
	q := datatug.QueryDefWithFolderPath{QueryDef: datatug.QueryDef{Type: datatug.QueryTypeSQL, Text: "SELECT name FROM person", SemanticModel: &model}}
	q.ID = "people"
	q.Title = "People"
	ctx := context.Background()
	first, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadQueryRevision(ctx, q.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.Query.SemanticModel, &model) {
		t.Fatal("semantic documents changed")
	}
	q.SemanticModel.Meaning.Concepts[1].Description = "Preferred name of a person"
	second, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision == first.Revision {
		t.Fatal("metadata revision unchanged")
	}
	if _, err = store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: first.Revision}); err == nil {
		t.Fatal("stale semantic write accepted")
	}
	q.SemanticModel.Bindings = []datatug.QuerySemanticBinding{}
	third, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: second.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Query.SemanticModel.Meaning.Concepts) != 2 {
		t.Fatal("unbinding removed meaning definitions")
	}
}

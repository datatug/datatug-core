package datatug

import (
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/test"
)

func newQueryDef(queryType, text string) QueryDef {
	return QueryDef{
		ProjectItem: ProjectItem{
			Access:  "public",
			UserIDs: []string{"user-id"},
			ProjItemBrief: ProjItemBrief{
				ID:     "test-id",
				Title:  "Test query",
				Folder: "~",
				ListOfTags: ListOfTags{
					Tags: []string{"tag1", "tag2"},
				},
			},
		},
		Type:  queryType,
		Draft: false,
		Text:  text,
	}
}

func TestQueryFolders_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := QueryFolders{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "f1"}}}}
		test.IsValidRecord(t, "valid", v)
	})
	t.Run("invalid_folder", func(t *testing.T) {
		v := QueryFolders{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "f1"}}, Folders: QueryFolders{{}}}}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("invalid_item", func(t *testing.T) {
		v := QueryFolders{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "f1"}}, Items: QueryDefs{{}}}}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

func TestQueryFolderBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := QueryFolderBrief{ProjItemBrief: ProjItemBrief{ID: "f1", Title: "T"}}
		test.IsValidRecord(t, "valid", v)
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := QueryFolderBrief{}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("invalid_subfolder", func(t *testing.T) {
		v := QueryFolderBrief{
			ProjItemBrief: ProjItemBrief{ID: "f1", Title: "T"},
			Folders:       []*QueryFolderBrief{{}},
		}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("invalid_item", func(t *testing.T) {
		v := QueryFolderBrief{
			ProjItemBrief: ProjItemBrief{ID: "f1", Title: "T"},
			Items:         []*QueryDefBrief{{}},
		}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

func TestQueryDefBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := QueryDefBrief{ProjItemBrief: ProjItemBrief{ID: "q1", Title: "T"}, Type: "SQL"}
		test.IsValidRecord(t, "valid", v)
	})
	t.Run("missing_type", func(t *testing.T) {
		v := QueryDefBrief{ProjItemBrief: ProjItemBrief{ID: "q1", Title: "T"}}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

func TestQueryDefWithFolderPath_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := QueryDefWithFolderPath{FolderPath: "~", QueryDef: newQueryDef("SQL", "SELECT 1")}
		test.IsValidRecord(t, "valid", v)
	})
	t.Run("missing_folder_path", func(t *testing.T) {
		v := QueryDefWithFolderPath{QueryDef: newQueryDef("SQL", "SELECT 1")}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

func TestQueryDef_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		queryDef := newQueryDef("SQL", "select * from users")
		test.IsValidRecord(t, "sql", queryDef)
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.IsInvalidRecord(t, "empty_record", QueryDef{})
	})
	t.Run("folder_with_text", func(t *testing.T) {
		v := newQueryDef("folder", "some text")
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("http_with_catalog", func(t *testing.T) {
		v := newQueryDef("HTTP", "")
		v.Targets = []QueryDefTarget{{Catalog: "c1"}}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("unsupported_type", func(t *testing.T) {
		v := newQueryDef("UNKNOWN", "")
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("invalid_parameters", func(t *testing.T) {
		v := newQueryDef("SQL", "SELECT 1")
		v.Parameters = Parameters{{ID: ""}}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

func TestQueryResult_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := QueryResult{Created: time.Now(), Driver: "mysql", Target: "t1"}
		test.IsValidRecord(t, "valid", v)
	})
	t.Run("zero_created", func(t *testing.T) {
		v := QueryResult{Driver: "mysql", Target: "t1"}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("missing_target", func(t *testing.T) {
		v := QueryResult{Created: time.Now(), Driver: "mysql"}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("missing_driver", func(t *testing.T) {
		v := QueryResult{Created: time.Now(), Target: "t1"}
		test.IsInvalidRecord(t, "invalid", v)
	})
	t.Run("invalid_recordset", func(t *testing.T) {
		v := QueryResult{Created: time.Now(), Driver: "mysql", Target: "t1", Recordsets: []Recordset{{Rows: [][]interface{}{nil}}}}
		test.IsInvalidRecord(t, "invalid", v)
	})
}

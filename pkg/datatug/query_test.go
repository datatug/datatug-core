package datatug

import (
	"testing"

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

func TestQueryDef_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		queryDef := newQueryDef("SQL", "select * from users")
		test.IsValidRecord(t, "sql", queryDef)
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.IsInvalidRecord(t, "empty_record", QueryDef{})
		//t.Run("invalid_folder", func(t *testing.T) {
		//	queryDef := newQueryDef("SQL", "select * from users")
		//	queryDef.Folder = ""
		//	test.IsInvalidRecord(t, "empty_folder", queryDef, func(t *testing.T, err error) {
		//		if !validation.IsBadFieldValueError(err) {
		//			t.Errorf("expected to get bad field value error, got %T: %v", err, err)
		//		}
		//	})
		//	queryDef.Folder = "///"
		//	test.IsInvalidRecord(t, "bad_folder", queryDef, func(t *testing.T, err error) {
		//		if !validation.IsBadFieldValueError(err) {
		//			t.Errorf("expected to get bad field value error, got %T: %v", err, err)
		//		}
		//	})
		//})
	})
}

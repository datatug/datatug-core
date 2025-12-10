package datatug

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/test"
)

func TestFolder_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		test.IsValidRecord(t, "good_folder", Folder{
			Name: "good folder",
		})
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.IsInvalidRecord(t, "empty_record", Folder{})
	})
}

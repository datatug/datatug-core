package models

import (
	"github.com/datatug/datatug/packages/test"
	"testing"
)

func TestFolder_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		test.ValidRecord(t, "good_folder", Folder{
			Name: "good folder",
		})
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.InvalidRecord(t, "empty_record", Folder{})
	})
}

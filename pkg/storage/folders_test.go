package storage

import (
	"github.com/datatug/datatug-core/pkg/test"
	"testing"
)

func TestCreateFolderRequest_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		test.IsValidRequest(t, "must_pass",
			CreateFolderRequest{
				Name: "New folder",
				Path: "~/public_folder",
			},
		)
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.IsInvalidRequest(t, "empty_request", CreateFolderRequest{})
		t.Run("invalid_name", func(t *testing.T) {
			test.IsInvalidRequest(t, "whitespace", CreateFolderRequest{Name: " \t "})
		})
		t.Run("invalid_note", func(t *testing.T) {
			test.IsInvalidRequest(t, "whitespace", CreateFolderRequest{Name: "new folder", Note: " "})
		})
	})
}

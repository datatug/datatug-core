package storage

import (
	"github.com/datatug/datatug/packages/test"
	"testing"
)

func TestCreateFolderRequest_Validate(t *testing.T) {
	t.Run("must_pass", func(t *testing.T) {
		test.ValidRequest(t, "must_pass",
			CreateFolderRequest{
				Name: "New folder",
				Path: "~/public_folder",
			},
		)
	})
	t.Run("must_return_error", func(t *testing.T) {
		test.InvalidRequest(t, "empty_request", CreateFolderRequest{})
		t.Run("invalid_name", func(t *testing.T) {
			test.InvalidRequest(t, "whitespace", CreateFolderRequest{Name: " \t "})
		})
		t.Run("invalid_note", func(t *testing.T) {
			test.InvalidRequest(t, "whitespace", CreateFolderRequest{Name: "new folder", Note: " "})
		})
	})
}

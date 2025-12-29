package storage

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/test"
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
		t.Run("invalid_path", func(t *testing.T) {
			test.IsInvalidRequest(t, "empty", CreateFolderRequest{Name: "new folder", Path: ""})
			test.IsInvalidRequest(t, "empty_segment_start", CreateFolderRequest{Name: "new folder", Path: "/a"})
			test.IsInvalidRequest(t, "empty_segment_middle", CreateFolderRequest{Name: "new folder", Path: "a//b"})
			test.IsInvalidRequest(t, "empty_segment_end", CreateFolderRequest{Name: "new folder", Path: "a/"})
		})
		t.Run("invalid_note", func(t *testing.T) {
			test.IsInvalidRequest(t, "whitespace", CreateFolderRequest{Name: "new folder", Path: "p", Note: " "})
		})
	})
}

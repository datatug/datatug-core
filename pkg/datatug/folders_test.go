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
	t.Run("invalid_name_spaces", func(t *testing.T) {
		test.IsInvalidRecord(t, "spaces", Folder{Name: " folder "})
	})
	t.Run("negative_number_of", func(t *testing.T) {
		test.IsInvalidRecord(t, "negative", Folder{Name: "f1", NumberOf: map[string]int{"x": -1}})
	})
	t.Run("zero_number_of_deleted", func(t *testing.T) {
		f := Folder{Name: "f1", NumberOf: map[string]int{"x": 0}}
		_ = f.Validate()
		if _, ok := f.NumberOf["x"]; ok {
			t.Errorf("expected zero value to be deleted from NumberOf")
		}
	})
}

func TestFolderBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := FolderBrief{Title: "f1"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("empty_title", func(t *testing.T) {
		v := FolderBrief{Title: ""}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("spaces", func(t *testing.T) {
		v := FolderBrief{Title: " f1 "}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestFolderItem_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := FolderItem{ID: "id1", Title: "t1"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("empty_id", func(t *testing.T) {
		v := FolderItem{ID: "", Title: "t1"}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("empty_title", func(t *testing.T) {
		v := FolderItem{ID: "id1", Title: ""}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("id_spaces", func(t *testing.T) {
		v := FolderItem{ID: " id1 ", Title: "t1"}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("title_spaces", func(t *testing.T) {
		v := FolderItem{ID: "id1", Title: " t1 "}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

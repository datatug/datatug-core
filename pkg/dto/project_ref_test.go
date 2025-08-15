package dto

import (
	"github.com/datatug/datatug-core/pkg/test"
	"testing"
)

func TestProjectRef_Validate(t *testing.T) {
	createValidProjectRef := func() ProjectRef {
		return ProjectRef{
			StoreID:   "store-id",
			ProjectID: "project-id",
		}
	}
	t.Run("must_return_nil", func(t *testing.T) {
		v := createValidProjectRef()
		test.IsValidRecord(t, "valid_record", v)
	})
	t.Run("returns_error", func(t *testing.T) {
		t.Run("if_empty", func(t *testing.T) {
			v := ProjectRef{}
			if err := v.Validate(); err == nil {
				t.Error("Expected to get an error for validation of empty ProjectRef")
			}
		})
		t.Run("StoreID", func(t *testing.T) {
			v := createValidProjectRef()
			v.StoreID = ""
			test.IsInvalidRequest(t, "missing", v)
		})
		t.Run("ProjectID", func(t *testing.T) {
			v := createValidProjectRef()
			v.ProjectID = ""
			test.IsInvalidRequest(t, "missing", v)
		})
	})
}

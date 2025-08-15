package dto

import "testing"

func TestProjectItemRef_Validate(t *testing.T) {
	t.Run("must_return_nil", func(t *testing.T) {
		v := ProjectItemRef{
			ID: "test-id",
			ProjectRef: ProjectRef{
				StoreID:   "store-id",
				ProjectID: "project-id",
			},
		}
		if err := v.Validate(); err != nil {
			t.Errorf("Validation expected to pass but got unexpected error: %v", err)
		}
	})
	t.Run("must_return_error", func(t *testing.T) {
		v := ProjectItemRef{}
		if err := v.Validate(); err == nil {
			t.Error("Expected to get an error for validation of empty ProjectItemRef")
		}
	})
}

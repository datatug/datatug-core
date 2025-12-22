package datatug

import "testing"

func TestListOfTags_Validate(t *testing.T) { // TODO: test error type & text
	t.Run("should_pass_validation_for_nil_tags", func(t *testing.T) {
		v := ListOfTags{
			Tags: nil,
		}
		if err := v.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("should_pass_validation_for_empty_slice_of_tags", func(t *testing.T) {
		v := ListOfTags{
			Tags: make([]string, 0),
		}
		if err := v.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("should_pass_validation_for_unique_set_of_tags", func(t *testing.T) {
		v := ListOfTags{
			Tags: []string{"one", "two", "three"},
		}
		if err := v.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("should_fail_on_duplicates", func(t *testing.T) {
		v := ListOfTags{
			Tags: []string{"one", "two", "two"},
		}
		if err := v.Validate(); err == nil {
			t.Error("expected to get an error, got nil")
		}
	})
	t.Run("should_fail_on_empty_tag", func(t *testing.T) {
		v := ListOfTags{Tags: []string{""}}
		if err := v.Validate(); err == nil {
			t.Error("expected error")
		}
	})
	t.Run("should_fail_on_too_long_tag", func(t *testing.T) {
		tag := ""
		for i := 0; i < MaxTagLength+1; i++ {
			tag += "a"
		}
		v := ListOfTags{Tags: []string{tag}}
		if err := v.Validate(); err == nil {
			t.Error("expected error")
		}
	})
}

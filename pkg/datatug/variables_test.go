package datatug

import "testing"

func TestVariables_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var o Variables
		if err := o.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
	t.Run("empty", func(t *testing.T) {
		var o Variables
		o.Vars = make(map[string]VarInfo, 0)
		if err := o.Validate(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

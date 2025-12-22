package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVariables_validateVarType(t *testing.T) {
	assert.Error(t, validateVarType(""))
	assert.NoError(t, validateVarType("str"))
	assert.NoError(t, validateVarType("int"))
	assert.Error(t, validateVarType("unknown"))
}

func TestVarSetting_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := VarSetting{Type: "str"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid_type", func(t *testing.T) {
		v := VarSetting{Type: "unknown"}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("valid_regexp", func(t *testing.T) {
		v := VarSetting{Type: "str", ValuePattern: "^a$"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid_regexp", func(t *testing.T) {
		v := VarSetting{Type: "str", ValuePattern: "["}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("valid_min_max", func(t *testing.T) {
		minVal, maxVal := 1, 2
		v := VarSetting{Type: "int", Min: &minVal, Max: &maxVal}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid_min_max", func(t *testing.T) {
		minVal, maxVal := 2, 1
		v := VarSetting{Type: "int", Min: &minVal, Max: &maxVal}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestVarInfo_Validate(t *testing.T) {
	t.Run("valid_str", func(t *testing.T) {
		v := VarInfo{Type: "str", Value: "val"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("valid_int", func(t *testing.T) {
		v := VarInfo{Type: "int", Value: "123"}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid_int", func(t *testing.T) {
		v := VarInfo{Type: "int", Value: "abc"}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

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
	t.Run("valid", func(t *testing.T) {
		v := Variables{Vars: map[string]VarInfo{"v1": {Type: "str"}}}
		if err := v.Validate(); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("invalid_name", func(t *testing.T) {
		v := Variables{Vars: map[string]VarInfo{" v1 ": {Type: "str"}}}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
	t.Run("invalid_var", func(t *testing.T) {
		v := Variables{Vars: map[string]VarInfo{"v1": {Type: "unknown"}}}
		if err := v.Validate(); err == nil {
			t.Fatal("expected error")
		}
	})
}

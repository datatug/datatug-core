package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParameterDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "string"}.Validate())
	})
	t.Run("missing_id", func(t *testing.T) {
		assert.Error(t, ParameterDef{Type: "string"}.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		assert.Error(t, ParameterDef{ID: "p1"}.Validate())
	})
	t.Run("valid_default_string", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "string", DefaultValue: "val"}.Validate())
	})
	t.Run("invalid_default_string", func(t *testing.T) {
		assert.Error(t, ParameterDef{ID: "p1", Type: "string", DefaultValue: 123}.Validate())
	})
	t.Run("valid_default_integer", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "integer", DefaultValue: 123}.Validate())
	})
	t.Run("valid_default_number", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "number", DefaultValue: 123.45}.Validate())
	})
	t.Run("valid_default_boolean", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "boolean", DefaultValue: true}.Validate())
	})
	t.Run("valid_default_bit", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "bit", DefaultValue: 1}.Validate())
	})
}

func TestParameters_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, Parameters{{ID: "p1", Type: "string"}}.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		assert.Error(t, Parameters{{ID: ""}}.Validate())
	})
}

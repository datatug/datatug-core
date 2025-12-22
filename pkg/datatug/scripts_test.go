package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAction_Validate(t *testing.T) {
	t.Run("valid_sql", func(t *testing.T) {
		assert.NoError(t, Action{Type: "sql"}.Validate())
	})
	t.Run("valid_http", func(t *testing.T) {
		assert.NoError(t, Action{Type: "http"}.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		assert.Error(t, Action{}.Validate())
	})
	t.Run("unsupported_type", func(t *testing.T) {
		assert.Error(t, Action{Type: "unknown"}.Validate())
	})
}

func TestActions_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, Actions{{Type: "sql"}}.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		assert.Error(t, Actions{{}}.Validate())
	})
}

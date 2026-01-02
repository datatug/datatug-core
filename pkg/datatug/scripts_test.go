package datatug

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAction_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		for i, at := range []string{"sql", "http"} {
			a := Action{Type: at}
			a.ID = "a" + strconv.Itoa(i)
			assert.NoError(t, a.Validate())
		}
	})
	t.Run("invalid", func(t *testing.T) {
		t.Run("missing_type", func(t *testing.T) {
			var a Action
			a.ID = "a0"
			assert.Error(t, a.Validate())
		})
		t.Run("unsupported_type", func(t *testing.T) {
			a := Action{Type: "unknown"}
			a.ID = "a0"
			assert.Error(t, a.Validate())
		})
	})
}

func TestActions_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		a1 := Action{Type: "sql"}
		a1.ID = "a1"

		a2 := Action{Type: "sql"}
		a2.ID = "a2"
		assert.NoError(t, Actions{&a1, &a2}.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		assert.Error(t, Actions{{}}.Validate())
	})
}

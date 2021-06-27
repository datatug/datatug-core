package routes

import (
	"github.com/julienschmidt/httprouter"
	"testing"
)

func TestRegisterDatatugWriteOnlyHandlers(t *testing.T) {
	t.Run("should_fail", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Fatal("a panic expected for nil router")
			}
		}()
		RegisterDatatugWriteOnlyHandlers("", nil, nil)
	})

	t.Run("should_pass", func(t *testing.T) {
		RegisterDatatugWriteOnlyHandlers("", httprouter.New(), nil)
	})
}

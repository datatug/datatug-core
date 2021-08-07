package models

import (
	"github.com/datatug/datatug/packages/test"
	"testing"
)

func TestQueryDef_Validate(t *testing.T) {
	t.Run("must_return_error", func(t *testing.T) {
		test.InvalidRecord(t, "empty_record", QueryDef{})
	})
}

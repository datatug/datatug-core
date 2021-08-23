package mssql

import (
	"context"
	"testing"
)

func TestTablesProvider_GetTables(t *testing.T) {
	v := tablesProvider{}
	ctx := context.Background()
	t.Run("panics on nil db", func(t *testing.T) {
		defer func() {
			if err := recover(); err == nil {
				t.Error("expected to panic if DB parameter is not supplied")
			}
		}()
		_, _ = v.GetTables(ctx, nil, "catalog", "schema")
	})
}

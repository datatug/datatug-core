package filestore

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestTableModelColumnsFile_Validate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		v := TableModelColumnsFile{}
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := TableModelColumnsFile{
			Columns: []*datatug.ColumnModel{
				{ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
			},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		v := TableModelColumnsFile{
			Columns: []*datatug.ColumnModel{
				{ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: ""}}},
			},
		}
		assert.Error(t, v.Validate())
	})
}

func TestDbModelFile_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbModelFile{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := DbModelFile{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: ""}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_environments", func(t *testing.T) {
		v := DbModelFile{
			ProjectItem:  datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "m1"}},
			Environments: datatug.DbModelEnvironments{{ID: ""}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestDbCatalogFile_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbCatalogFile{Driver: "postgres"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_driver", func(t *testing.T) {
		v := DbCatalogFile{Driver: ""}
		assert.Error(t, v.Validate())
	})
	t.Run("sqlite3_missing_path", func(t *testing.T) {
		v := DbCatalogFile{Driver: "sqlite3", Path: ""}
		assert.Error(t, v.Validate())
	})
	t.Run("sqlite3_with_path", func(t *testing.T) {
		v := DbCatalogFile{Driver: "sqlite3", Path: "/path/to/db"}
		assert.NoError(t, v.Validate())
	})
}

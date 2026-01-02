package datatug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDbCatalogBase_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbCatalogBase{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}},
			Driver:      "mysql",
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := DbCatalogBase{Driver: "mysql"}
		assert.Error(t, v.Validate())
	})
	t.Run("missing_driver", func(t *testing.T) {
		v := DbCatalogBase{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("sqlite_missing_path", func(t *testing.T) {
		v := DbCatalogBase{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}},
			Driver:      "sqlite3",
		}
		assert.Error(t, v.Validate())
	})
}

func TestDbCatalog_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := DbCatalog{
			DbCatalogBase: DbCatalogBase{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}},
				Driver:      "mysql",
			},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_base", func(t *testing.T) {
		v := DbCatalog{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_schemas", func(t *testing.T) {
		v := DbCatalog{
			DbCatalogBase: DbCatalogBase{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "c1"}},
				Driver:      "mysql",
			},
			Schemas: DbSchemas{{}},
		}
		assert.Error(t, v.Validate())
	})
}

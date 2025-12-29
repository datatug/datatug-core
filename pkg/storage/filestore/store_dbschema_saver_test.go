package filestore

import (
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestDbSchemaSaver(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_dbschema")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	projectID := "test_project"
	projectPath := path.Join(tmpDir, projectID)

	dbServer := datatug.ServerReference{
		Driver: "sqlserver",
		Host:   "localhost",
	}

	catalogID := "db1"
	fsProjectStore := newFsProjectStore(projectID, projectPath)
	fsServerStore := fsDbServerStore{
		dbServer: dbServer,
		fsDbServersStore: fsDbServersStore{
			fsProjectStoreRef: fsProjectStoreRef{
				fsProjectStore: fsProjectStore,
			},
		},
	}

	catalogsStore := newFsDbCatalogsStore(fsServerStore)
	store := catalogsStore.DbCatalog(catalogID).(fsDbCatalogStore)

	t.Run("SaveDbSchemas", func(t *testing.T) {
		schemas := []*datatug.DbSchema{
			{
				ProjectItem: datatug.ProjectItem{
					ProjItemBrief: datatug.ProjItemBrief{
						ID: "dbo",
					},
				},
				Tables: []*datatug.CollectionInfo{
					{
						DBCollectionKey: datatug.NewTableKey("table1", "dbo", catalogID, nil),
						TableProps: datatug.TableProps{
							DbType: "BASE TABLE",
						},
					},
				},
			},
		}

		ctx := saveDbServerObjContext{
			catalog:    catalogID,
			dbServer:   datatug.ProjDbServer{Server: dbServer},
			repository: &datatug.ProjectRepository{},
			dirPath:    path.Join(projectPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Driver, dbServer.Host, DbCatalogsFolder, catalogID),
		}

		err := store.saveDbSchemas(schemas, ctx)
		assert.NoError(t, err)

		tablePath := path.Join(ctx.dirPath, SchemasFolder, "dbo", TablesFolder, "table1")
		assert.DirExists(t, tablePath)
		assert.FileExists(t, path.Join(tablePath, "dbo.table1.json"))
	})
}

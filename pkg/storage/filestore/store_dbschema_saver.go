package filestore

import (
	"fmt"
	"log"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
)

func (store fsDbCatalogStore) saveDbSchemas(schemas []*datatug.DbSchema, dbServerSaverCtx saveDbServerObjContext) error {
	return saveItems("schemas", len(schemas), func(i int) func() error {
		return func() error {
			schema := schemas[i]
			schemaCtx := dbServerSaverCtx
			schemaCtx.plural = "schemas"
			schemaCtx.dirPath = path.Join(dbServerSaverCtx.dirPath, storage.SchemasFolder, schema.ID)
			return store.saveDbSchema(schema, schemaCtx)
		}
	})
}

func (store fsDbCatalogStore) saveDbSchema(dbSchema *datatug.DbSchema, dbServerSaverCtx saveDbServerObjContext) error {
	log.Printf("Save DB schema [%v] for %v @ %v...", dbSchema.ID, dbServerSaverCtx.catalog, dbServerSaverCtx.dbServer.ID)
	err := parallel.Run(
		func() error {
			tablesCtx := dbServerSaverCtx
			tablesCtx.plural = storage.TablesFolder
			return store.saveTables(dbSchema.Tables, tablesCtx)
		},
		func() error {
			viewsCtx := dbServerSaverCtx
			viewsCtx.plural = storage.ViewsFolder
			return store.saveTables(dbSchema.Views, viewsCtx)
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB schema [%v]: %w", dbSchema.ID, err)
	}
	log.Printf("Saved DB schema [%v] for %v @ %v.", dbSchema.ID, dbServerSaverCtx.catalog, dbServerSaverCtx.dbServer.ID)
	return nil
}

func (store fsDbCatalogStore) saveTables(tables []*datatug.CollectionInfo, save saveDbServerObjContext) error {
	save.dirPath = path.Join(save.dirPath, save.plural)
	if len(tables) > 0 {
		if err := os.MkdirAll(save.dirPath, 0777); err != nil {
			return fmt.Errorf("failed to create a folder for %v: %w", save.plural, err)
		}
	}
	// TODO: Remove tables that does not exist anymore
	return saveItems("tables", len(tables), func(i int) func() error {
		return func() error {
			return store.saveTable(tables[i], save)
		}
	})
}

func (store fsDbCatalogStore) saveTable(table *datatug.CollectionInfo, save saveDbServerObjContext) (err error) {
	save.dirPath = path.Join(save.dirPath, table.Name())
	if err = os.MkdirAll(save.dirPath, 0777); err != nil {
		return err
	}

	var filePrefix string
	if table.Schema() == "" {
		filePrefix = table.Name()
	} else {
		filePrefix = fmt.Sprintf("%s.%s", table.Schema(), table.Name())
	}

	workers := make([]func() error, 0, 9)

	tableFile := TableFile{
		TableProps:   table.TableProps,
		PrimaryKey:   table.PrimaryKey,
		ForeignKeys:  table.ForeignKeys,
		ReferencedBy: table.ReferencedBy,
		Columns:      table.Columns,
		Indexes:      table.Indexes,
	}

	workers = append(workers, saveToFile(save.dirPath, fmt.Sprintf("%v.json", filePrefix), tableFile))
	workers = append(workers, store.writeTableReadme(table, save))

	return parallel.Run(workers...)
}

func (store fsDbCatalogStore) writeTableReadme(table *datatug.CollectionInfo, save saveDbServerObjContext) func() error {
	return func() error {
		//log.Printf("Saving readme.md for table %v.%v.%v...\n", catalog, table.Schema, table.Name)
		file, _ := os.Create(path.Join(save.dirPath, "README.md"))
		defer func() {
			_ = file.Close()
		}()
		if err := store.readmeEncoder.TableToReadme(file, save.repository, save.catalog, table, save.dbServer); err != nil {
			return err
		}
		return nil
	}
}

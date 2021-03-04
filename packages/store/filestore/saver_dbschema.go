package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"path"
)

func (s fileSystemSaver) saveDbSchemas(schemas []*models.DbSchema, dbServerSaverCtx saveDbServerObjContext) error {
	return s.saveItems("schemas", len(schemas), func(i int) func() error {
		return func() error {
			schema := schemas[i]
			schemaCtx := dbServerSaverCtx
			schemaCtx.plural = "schemas"
			schemaCtx.dirPath = path.Join(dbServerSaverCtx.dirPath, SchemasFolder, schema.ID)
			return s.saveDbSchema(schema, schemaCtx)
		}
	})
}

func (s fileSystemSaver) saveDbSchema(dbSchema *models.DbSchema, dbServerSaverCtx saveDbServerObjContext) error {
	log.Printf("Save DB schema [%v] for %v @ %v...", dbSchema.ID, dbServerSaverCtx.catalog, dbServerSaverCtx.dbServer.ID)
	err := parallel.Run(
		func() error {
			tablesCtx := dbServerSaverCtx
			tablesCtx.plural = TablesFolder
			return s.saveTables(dbSchema.Tables, tablesCtx)
		},
		func() error {
			viewsCtx := dbServerSaverCtx
			viewsCtx.plural = ViewsFolder
			return s.saveTables(dbSchema.Views, viewsCtx)
		},
	)
	if err != nil {
		return fmt.Errorf("failed to save DB schema [%v]: %w", err)
	}
	log.Printf("Saved DB schema [%v] for %v @ %v.", dbSchema.ID, dbServerSaverCtx.catalog, dbServerSaverCtx.dbServer.ID)
	return nil
}

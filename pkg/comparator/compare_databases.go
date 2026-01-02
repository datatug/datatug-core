package comparator

import (
	"fmt"
	"log"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
)

// DatabasesToCompare defines databases to compare
type DatabasesToCompare struct {
	DbModel      datatug.DbModel
	Environments []EnvToCompare
}

// EnvToCompare defines env to compare
type EnvToCompare struct {
	ID        string
	Databases datatug.DbCatalogs
}

type schemaToCompare struct { // should it be rather called `schemaToCompare`?
	envID       string
	dbID        string
	schemaID    string // schema GetID
	schemaModel *datatug.Schema
	dbSchemas   []*datatug.DbSchema
}

type tableToCompare struct {
	envID      string
	dbID       string
	tableName  string
	tableModel *datatug.TableModel
	dbTables   []*datatug.CollectionInfo
}

// CompareDatabases compares databases
//
//goland:noinspection GoUnusedExportedFunction
func CompareDatabases(dbsToCompare DatabasesToCompare) (dbDifferences datatug.DatabaseDifferences, err error) {
	return dbDifferences, compareSchemas(dbsToCompare, &dbDifferences)
}

func compareSchemas(dbs DatabasesToCompare, dbDifferences *datatug.DatabaseDifferences) (err error) {
	var targets []*schemaToCompare

	for _, env := range dbs.Environments {
		for _, db := range env.Databases {
			for _, schema := range db.Schemas {
				if schema == nil {
					continue
				}
				var target *schemaToCompare
				for _, t := range targets {
					if t.schemaID == schema.ID {
						target = t
						break
					}
				}
				if target == nil {
					target = &schemaToCompare{
						envID:       env.ID,
						dbID:        db.ID,
						schemaID:    schema.ID,
						schemaModel: dbs.DbModel.Schemas.GetByID(schema.ID),
					}
					targets = append(targets, target)
				}
				target.dbSchemas = append(target.dbSchemas, schema)
			}
		}
	}
	workers := make([]func() error, len(targets))

	dbDifferences.SchemasDiff = make(datatug.SchemasDiff, len(targets))

	for i, target := range targets {
		workers[i] = func() (err error) {
			if dbDifferences.SchemasDiff[i], err = compareSchema(*target); err != nil {
				return fmt.Errorf("failed to compare schema [%v]: %w", target.dbSchemas, err)
			}
			return
		}
	}

	return parallel.Run(workers...)
}

func compareSchema(target schemaToCompare) (schemaDiff datatug.SchemaDiff, err error) {
	err = parallel.Run( // TODO(performance): measure if it make sense to run in parallel on typical payload
		func() (err error) { // compare tables
			schemaDiff.TablesDiff, err = compareTables(
				target,
				func(schemaModel *datatug.Schema) datatug.TableModels {
					return schemaModel.Tables
				},
				func(schema *datatug.DbSchema) datatug.Tables {
					return schema.Tables
				},
			)
			return
		},
		func() (err error) { // compare views
			schemaDiff.ViewsDiff, err = compareTables(
				target,
				func(schemaModel *datatug.Schema) datatug.TableModels {
					return schemaModel.Views
				},
				func(schema *datatug.DbSchema) datatug.Tables {
					return schema.Views
				},
			)
			return
		},
	)
	return
}

func compareTables(target schemaToCompare, getTableModels func(schemaModel *datatug.Schema) datatug.TableModels, getDbTables func(db *datatug.DbSchema) datatug.Tables) (tablesDiff datatug.TablesDiff, err error) {
	var tablesToCompare []tableToCompare
	var tableModels datatug.TableModels
	if target.schemaModel != nil {
		tableModels = getTableModels(target.schemaModel)
	}

	for _, dbSchema := range target.dbSchemas {
		dbTables := getDbTables(dbSchema)
	DbTables:
		for _, dbTable := range dbTables {
			if dbTable == nil {
				continue
			}
			for _, t2c := range tablesToCompare {
				if t2c.tableName == dbTable.Name() {
					t2c.dbTables = append(t2c.dbTables, dbTable)
					continue DbTables
				}
			}
			tableName := dbTable.Name()
			tablesToCompare = append(tablesToCompare, tableToCompare{
				tableName:  tableName,
				tableModel: tableModels.GetByName(tableName),
			})
		}
	}
	tablesDiff = make(datatug.TablesDiff, len(tablesToCompare))
	for i, t2c := range tablesToCompare {
		if tablesDiff[i], err = compareTable(target.schemaID, t2c); err != nil {
			return
		}
	}
	return
}

func compareTable(schemaID string, toCompare tableToCompare) (tablesDiff datatug.TableDiff, err error) {
	log.Printf("Comparing %v: %v.%v.%v.", toCompare.envID, toCompare.dbID, schemaID, toCompare.tableName)
	if toCompare.tableName == "$error$" {
		return tablesDiff, fmt.Errorf("intentional error for testing")
	}
	return
}

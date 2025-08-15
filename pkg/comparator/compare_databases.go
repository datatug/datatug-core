package comparator

import (
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/parallel"
	"log"
)

// DatabasesToCompare defines databases to compare
type DatabasesToCompare struct {
	DbModel      models.DbModel
	Environments []EnvToCompare
}

// EnvToCompare defines env to compare
type EnvToCompare struct {
	ID        string
	Databases models.DbCatalogs
}

type schemaToCompare struct { // should it be rather called `schemaToCompare`?
	envID       string
	dbID        string
	schemaID    string // schema ID
	schemaModel *models.SchemaModel
	dbSchemas   []*models.DbSchema
}

type tableToCompare struct {
	envID      string
	dbID       string
	tableName  string
	tableModel *models.TableModel
	dbTables   []*models.Table
}

// CompareDatabases compares databases
//
//goland:noinspection GoUnusedExportedFunction
func CompareDatabases(dbsToCompare DatabasesToCompare) (dbDifferences models.DatabaseDifferences, err error) {
	return dbDifferences, compareSchemas(dbsToCompare, &dbDifferences)
}

func compareSchemas(dbs DatabasesToCompare, dbDifferences *models.DatabaseDifferences) (err error) {
	var targets []*schemaToCompare

	for _, env := range dbs.Environments {
		for _, db := range env.Databases {
			for _, schema := range db.Schemas {
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
			}
		}
	}
	workers := make([]func() error, len(targets))

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

func compareSchema(target schemaToCompare) (schemaDiff models.SchemaDiff, err error) {
	err = parallel.Run( // TODO(performance): measure if it make sense to run in parallel on typical payload
		func() (err error) { // compare tables
			schemaDiff.TablesDiff, err = compareTables(
				target,
				func(schemaModel *models.SchemaModel) models.TableModels {
					return schemaModel.Tables
				},
				func(schema *models.DbSchema) models.Tables {
					return schema.Tables
				},
			)
			return
		},
		func() (err error) { // compare views
			schemaDiff.ViewsDiff, err = compareTables(
				target,
				func(schemaModel *models.SchemaModel) models.TableModels {
					return schemaModel.Views
				},
				func(schema *models.DbSchema) models.Tables {
					return schema.Views
				},
			)
			return
		},
	)
	return
}

func compareTables(target schemaToCompare, getTableModels func(schemaModel *models.SchemaModel) models.TableModels, getDbTables func(db *models.DbSchema) models.Tables) (tablesDiff models.TablesDiff, err error) {
	var tablesToCompare []tableToCompare
	tableModels := getTableModels(target.schemaModel)

	for _, dbSchema := range target.dbSchemas {
		dbTables := getDbTables(dbSchema)
	DbTables:
		for _, dbTable := range dbTables {
			for _, t2c := range tablesToCompare {
				if t2c.tableName == dbTable.Name {
					t2c.dbTables = append(t2c.dbTables, dbTable)
					continue DbTables
				}
			}
			tablesToCompare = append(tablesToCompare, tableToCompare{
				tableName:  dbTable.Name,
				tableModel: tableModels.GetByName(dbTable.Name),
			})
		}
	}
	tablesDiff = make(models.TablesDiff, len(tablesToCompare))
	for i, t2c := range tablesToCompare {
		if tablesDiff[i], err = compareTable(target.schemaID, t2c); err != nil {
			return
		}
	}
	return
}

func compareTable(schemaID string, toCompare tableToCompare) (tablesDiff models.TableDiff, err error) {
	log.Printf("Comparing %v: %v.%v.%v.", toCompare.envID, toCompare.dbID, schemaID, toCompare.tableName)
	return
}

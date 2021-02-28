package schemer

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"strings"
	"time"
)

func NewScanner(schemaProvider SchemaProvider) Scanner {
	return scanner{schemaProvider: schemaProvider}
}

type scanner struct {
	schemaProvider SchemaProvider
}

func (s scanner) ScanCatalog(c context.Context, db *sql.DB, name string) (database *models.DbCatalog, err error) {
	database = &models.DbCatalog{
		ProjectItem: models.ProjectItem{ID: name},
	}
	if err = s.scanTables(c, db, database); err != nil {
		return database, fmt.Errorf("failed to get tables & views: %w", err)
	}
	log.Println("Scanner completed tables scan.")
	return
}

func (s scanner) scanTables(c context.Context, db *sql.DB, database *models.DbCatalog) error {
	var tables []*models.Table
	tablesReader, err := s.schemaProvider.GetTables(c, db, database.ID, "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var workers = []func() error{
		func() error {
			if err = s.scanColumns(c, db, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive columns metadata: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.scanConstraints(c, db, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive constraints metadata: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.scanIndexes(c, db, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive indexes metadata: %w", err)
			}
			return nil
		},
	}
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		t, err := tablesReader.NextTable()
		if err != nil {
			return err
		}
		if t == nil {
			break
		}
		tables = append(tables, t)
		schema := database.Schemas.GetByID(t.Schema)
		if schema == nil {
			schema = &models.DbSchema{ProjectItem: models.ProjectItem{ID: t.Schema}}
			database.Schemas = append(database.Schemas, schema)
		}
		switch t.DbType {
		case "BASE TABLE":
			schema.Tables = append(schema.Tables, t)
			workers = append(workers, func() (err error) {
				t.RecordsCount, err = s.schemaProvider.RecordsCount(c, db, database.ID, t.Schema, t.Name)
				if err != nil {
					log.Printf("failed to retiever records count for %v.%v.%v: %v", database.ID, t.Schema, t.Name, err)
					//return fmt.Errorf()
				}
				return nil
			})
		case "VIEW":
			schema.Views = append(schema.Views, t)
		default:
			return fmt.Errorf("object [%v] has unknown DB type: %v", t.Name, t.DbType)
		}
	}
	err = parallel.Run(workers...)
	return err
}

func (s scanner) scanColumns(c context.Context, db *sql.DB, catalog string, tablesFinder sortedTables) error {
	if s.schemaProvider.IsBulkProvider() {
		return s.scanColumnsInBulk(c, db, catalog, tablesFinder)
	}
	return s.scanColumnsByTable(c, db, catalog, tablesFinder.tables)
}

func (s scanner) scanColumnsByTable(c context.Context, db *sql.DB, catalog string, tables []*models.Table) error {
	workers := make([]func() error, len(tables))
	for i, t := range tables {
		workers[i] = func() error {
			return s.scanTableCols(c, db, catalog, t)
		}
	}
	return parallel.Run(workers...)
}

func (s scanner) scanTableCols(c context.Context, db *sql.DB, catalog string, table *models.Table) error {
	columnsReader, err := s.schemaProvider.GetColumns(c, db, catalog, table.Schema, table.Name)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		column, err := columnsReader.NextColumn()
		if err != nil {
			return err
		}
		if column.Name == "" {
			return nil
		}
		table.Columns = append(table.Columns, &column.TableColumn)
	}
}

func (s scanner) scanColumnsInBulk(c context.Context, db *sql.DB, catalog string, tablesFinder sortedTables) error {
	columnsReader, err := s.schemaProvider.GetColumns(c, db, catalog, "", "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		column, err := columnsReader.NextColumn()
		if err != nil {
			return err
		}
		if column.Name == "" {
			return nil
		}
		if table := tablesFinder.SequentialFind(catalog, column.SchemaName, column.TableName); table != nil {
			table.Columns = append(table.Columns, &column.TableColumn)
		} else {
			return fmt.Errorf("unknown table referenced by column [%v]: %v.%v.%v",
				column.Name, catalog, column.SchemaName, column.TableName)
		}
	}
}

func (s scanner) scanIndexes(c context.Context, db *sql.DB, catalog string, tablesFinder sortedTables) error {
	reader, err := s.schemaProvider.GetIndexes(c, db, catalog, "", "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var indexes []Index
	for i := 0; ; i++ {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		index, err := reader.NextIndex()
		if err != nil {
			return err
		}
		if index.Index == nil {
			break
		}
		indexes = append(indexes, index)
		if index.Name == "" {
			return fmt.Errorf("got index with an empty name at iteration #%v", i)
		}
		table := tablesFinder.SequentialFind(catalog, index.SchemaName, index.TableName)
		if table == nil {
			return fmt.Errorf("unknown table referenced by constraint [%v]: %v.%v.%v",
				index.Name, catalog, index.SchemaName, index.TableName)
		}
		table.Indexes = append(table.Indexes, index.Index)
	}
	if err = s.scanIndexColumns(c, db, catalog, sortedIndexes{indexes: indexes}); err != nil {
		return fmt.Errorf("failed to retrieve index columns: %v", err)
	}
	return nil
}

func (s scanner) scanIndexColumns(c context.Context, db *sql.DB, catalog string, indexFinder sortedIndexes) error {
	reader, err := s.schemaProvider.GetIndexColumns(c, db, catalog, "", "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for i := 0; ; i++ {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		indexColumn, err := reader.NextIndexColumn()
		if err != nil {
			return fmt.Errorf("failed to get next index column at iteration #%v: %w", i, err)
		}
		if indexColumn.IndexColumn == nil {
			break
		}
		index := indexFinder.SequentialFind(indexColumn.SchemaName, indexColumn.TableName, indexColumn.IndexName)
		if index.Index == nil {
			indexNames := make([]string, len(indexFinder.indexes))
			for k, index := range indexFinder.indexes {
				indexNames[k] = index.Name
			}
			return fmt.Errorf("unknown index referenced by column [%v.%v.%v.%v] at iteration #%v: %v\nKnown indexes: %v",
				catalog, indexColumn.SchemaName, indexColumn.TableName, indexColumn.Name, i, indexColumn.IndexName, strings.Join(indexNames, ", "))
		}
		index.Columns = append(index.Columns, indexColumn.IndexColumn)
	}
	return nil
}

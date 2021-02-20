package schemer

import (
	"context"
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

func (s scanner) ScanCatalog(c context.Context, name string) (database *models.DbCatalog, err error) {
	database = &models.DbCatalog{
		ProjectItem: models.ProjectItem{ID: name},
	}
	if err = s.scanTables(c, database); err != nil {
		return database, fmt.Errorf("failed to get tables & views: %w", err)
	}
	log.Println("Scanner completed tables scan.")
	return
}

func (s scanner) scanTables(c context.Context, database *models.DbCatalog) error {
	var tables []*models.Table
	tablesReader, err := s.schemaProvider.Objects(c, database.ID)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		t, err := tablesReader.NextObject()
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
		case "VIEW":
			schema.Views = append(schema.Views, t)
		default:
			return fmt.Errorf("object [%v] has unknown DB type: %v", t.Name, t.DbType)
		}
	}
	err = parallel.Run(
		func() error {
			if err = s.scanColumns(c, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive columns metadata: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.scanConstraints(c, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive constraints metadata: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.scanIndexes(c, database.ID, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive indexes metadata: %w", err)
			}
			return nil
		},
	)
	return err
}

func (s scanner) scanColumns(c context.Context, catalog string, tablesFinder sortedTables) error {
	columnsReader, err := s.schemaProvider.Columns(c, catalog)
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



func (s scanner) scanIndexes(c context.Context, catalog string, tablesFinder sortedTables) error {
	reader, err := s.schemaProvider.Indexes(c, catalog)
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
	if err = s.scanIndexColumns(c, catalog, sortedIndexes{indexes: indexes}); err != nil {
		return fmt.Errorf("failed to retrieve index columns: %v", err)
	}
	return nil
}

func (s scanner) scanIndexColumns(c context.Context, catalog string, indexFinder sortedIndexes) error {
	reader, err := s.schemaProvider.IndexColumns(c, catalog)
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

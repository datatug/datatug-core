package schemer

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"time"
)

func NewScanner(schemaProvider SchemaProvider) Scanner {
	return scanner{schemaProvider: schemaProvider}
}

type scanner struct {
	schemaProvider SchemaProvider
}

func (s scanner) ScanCatalog(c context.Context, name string) (database *models.Database, err error) {
	database = &models.Database{
		ProjectItem: models.ProjectItem{ID: name},
	}
	if err = s.scanTables(c, database); err != nil {
		return database, fmt.Errorf("failed to get tables & views: %w", err)
	}
	log.Println("GetDatabase completed")
	return
}

func (s scanner) scanTables(c context.Context, database *models.Database) error {
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

func (s scanner) scanConstraints(c context.Context, catalog string, tablesFinder sortedTables) error {
	reader, err := s.schemaProvider.Constraints(c, catalog)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		constraint, err := reader.NextConstraint()
		if err != nil {
			return err
		}
		if constraint.Name == "" {
			break
		}
		table := tablesFinder.SequentialFind(catalog, constraint.SchemaName, constraint.TableName)
		if table == nil {
			return fmt.Errorf("unknown table referenced by constraint [%v]: %v.%v.%v",
				constraint.Name, catalog, constraint.SchemaName, constraint.TableName)
		}
	}
	return nil
}

func (s scanner) scanIndexes(c context.Context, catalog string, tablesFinder sortedTables) error {
	reader, err := s.schemaProvider.Indexes(c, catalog)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var indexes []Index
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		index, err := reader.NextIndex()
		if err != nil {
			return err
		}
		indexes = append(indexes, index)
		if index.Name == "" {
			break
		}
		table := tablesFinder.SequentialFind(catalog, index.SchemaName, index.TableName)
		if table == nil {
			return fmt.Errorf("unknown table referenced by constraint [%v]: %v.%v.%v",
				index.Name, catalog, index.SchemaName, index.TableName)
		}
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
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		indexColumn, err := reader.NextIndexColumn()
		if err != nil {
			return err
		}
		if indexColumn.Name == "" {
			break
		}
		index := indexFinder.SequentialFind(indexColumn.SchemaName, indexColumn.TableName, indexColumn.Name)
		if index.Index == nil {
			return fmt.Errorf("unknown index referenced by column [%v.%v.%v.%v]: %v",
				catalog, indexColumn.SchemaName, indexColumn.TableName, indexColumn.Name, indexColumn.IndexName)
		}
	}
	return nil
}

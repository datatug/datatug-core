package schemer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
)

// NewScanner creates new scanner
func NewScanner(schemaProvider SchemaProvider) Scanner {
	return scanner{schemaProvider: schemaProvider}
}

type scanner struct {
	schemaProvider SchemaProvider
}

func (s scanner) ScanCatalog(c context.Context, name string) (dbCatalog *datatug.DbCatalog, err error) {
	dbCatalog = new(datatug.DbCatalog)
	dbCatalog.ID = name
	if err = s.scanTables(c, dbCatalog); err != nil {
		return dbCatalog, fmt.Errorf("failed to get Tables & views: %w", err)
	}
	log.Println("Scanner completed Tables scan.")
	return
}

func (s scanner) scanTables(c context.Context, catalog *datatug.DbCatalog) error {
	var tables []*datatug.CollectionInfo
	tablesReader, err := s.schemaProvider.GetCollections(c, NewSchemaKey(catalog.ID, ""))
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var workers []func() error
	if s.schemaProvider.IsBulkProvider() {
		workers = append(workers,
			func() error {
				if err = s.scanColumnsInBulk(c, catalog.ID, SortedTables{Tables: tables}); err != nil {
					return fmt.Errorf("failed to retrieve columns metadata: %w", err)
				}
				return nil
			},
			func() error {
				if err = s.scanConstraintsInBulk(c, catalog.ID, SortedTables{Tables: tables}); err != nil {
					return fmt.Errorf("failed to retrieve constraints metadata: %w", err)
				}
				return nil
			},
			func() error {
				if err = s.scanIndexesInBulk(c, catalog.ID, SortedTables{Tables: tables}); err != nil {
					return fmt.Errorf("failed to retrieve indexes metadata: %w", err)
				}
				return nil
			},
		)
	}
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		t, err := tablesReader.NextCollection()
		if err != nil {
			return err
		}
		if t == nil {
			break
		}
		tables = append(tables, t)
		schema := catalog.Schemas.GetByID(t.Schema)
		if schema == nil {
			schema = new(datatug.DbSchema)
			schema.ID = t.Schema
			catalog.Schemas = append(catalog.Schemas, schema)
		}
		switch t.DbType {
		case "BASE TABLE":
			schema.Tables = append(schema.Tables, t)
			workers = append(workers, func() (err error) {
				t.RecordsCount, err = s.schemaProvider.RecordsCount(c, catalog.ID, t.Schema, t.Name)
				if err != nil {
					log.Printf("failed to retiever records count for %v.%v.%v: %v", catalog.ID, t.Schema, t.Name, err)
					//return fmt.Errorf()
				}
				return nil
			})
		case "VIEW":
			schema.Views = append(schema.Views, t)
		default:
			return fmt.Errorf("object [%v] has unknown DB type: %v", t.Name, t.DbType)
		}
		if !s.schemaProvider.IsBulkProvider() {
			workers = append(workers, func() error {
				return s.getTableProps(c, catalog.ID, t)
			})
		}
	}
	err = parallel.Run(workers...)
	if !s.schemaProvider.IsBulkProvider() {
		for _, table := range tables {
			if err = s.scanTableConstraints(c, catalog.ID, table, tables); err != nil {
				return err
			}
		}
	}
	return err
}

func (s scanner) scanColumnsInBulk(c context.Context, catalog string, tablesFinder SortedTables) error {
	columnsReader, err := s.schemaProvider.GetColumnsReader(c, catalog, ColumnsFilter{})
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
			table.Columns = append(table.Columns, &column.ColumnInfo)
		} else {
			return fmt.Errorf("unknown table referenced by column [%v]: %v.%v.%v",
				column.Name, catalog, column.SchemaName, column.TableName)
		}
	}
}

func (s scanner) scanIndexesInBulk(c context.Context, catalog string, tablesFinder SortedTables) error {
	reader, err := s.schemaProvider.GetIndexes(c, catalog, "", "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var indexes []*Index
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
	if err = s.scanIndexColumnsInBulk(c, catalog, SortedIndexes{indexes: indexes}); err != nil {
		return fmt.Errorf("failed to retrieve index columns: %v", err)
	}
	return nil
}

func (s scanner) scanIndexColumnsInBulk(c context.Context, catalog string, indexFinder SortedIndexes) error {
	reader, err := s.schemaProvider.GetIndexColumns(c, catalog, "", "", "")
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

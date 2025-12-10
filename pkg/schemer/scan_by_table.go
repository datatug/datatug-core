package schemer

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
)

func (s scanner) getTableProps(c context.Context, catalog string, table *datatug.CollectionInfo) error {
	log.Printf("getTableProps() table=%v", table.Name)
	err := parallel.Run(
		func() (err error) {
			if err = s.scanTableCols(c, catalog, table); err != nil {
				return fmt.Errorf("failed to get table columns: %w", err)
			}
			return nil
		},
		func() (err error) {
			if err = s.scanTableIndexes(c, catalog, table); err != nil {
				return fmt.Errorf("failed to get table indexes: %w", err)
			}
			return nil
		},
	)
	if err != nil {
		return fmt.Errorf("failed to get table props: %w", err)
	}
	return nil
}

func (s scanner) scanTableCols(c context.Context, catalog string, table *datatug.CollectionInfo) error {
	log.Printf("scanning columns for table %v...", table.Name)
	columnsReader, err := s.schemaProvider.GetColumns(c, catalog, table.Schema, table.Name)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	pkColumns := make(datatug.TableColumns, 0, 8)
	defer func() {
		if len(pkColumns) > 0 {
			sort.Sort(pkColumns.ByPrimaryKeyPosition())
			table.PrimaryKey = &datatug.UniqueKey{
				Name: "PK_" + table.Name,
			}
			for _, c := range pkColumns {
				table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, c.Name)
			}
		}
	}()
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
		table.Columns = append(table.Columns, &column.ColumnInfo)
		if column.PrimaryKeyPosition > 0 {
			pkColumns = append(pkColumns, &column.ColumnInfo)
		}
	}
}

func (s scanner) scanTableIndexes(c context.Context, catalog string, table *datatug.CollectionInfo) error {
	indexesReader, err := s.schemaProvider.GetIndexes(c, catalog, table.Schema, table.Name)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	var workers []func() error
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		index, err := indexesReader.NextIndex()
		if err != nil {
			return fmt.Errorf("failed to get index record: %w", err)
		}
		if index == nil {
			break
		}
		table.Indexes = append(table.Indexes, index.Index)
		workers = append(workers, func() error {
			if err := s.scanIndexColumns(c, catalog, table, index.Index); err != nil {
				return fmt.Errorf("failed to get columns of index [%v]: %w", index.Name, err)
			}
			return nil
		})
	}
	if err = parallel.Run(workers...); err != nil {
		return fmt.Errorf("failed to get index details: %w", err)
	}
	return nil
}

func (s scanner) scanIndexColumns(c context.Context, catalog string, table *datatug.CollectionInfo, index *datatug.Index) error {
	indexColumnsReader, err := s.schemaProvider.GetIndexColumns(c, catalog, table.Schema, table.Name, index.Name)
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		indexCol, err := indexColumnsReader.NextIndexColumn()
		if err != nil {
			return fmt.Errorf("failed to get index column record: %w", err)
		}
		if indexCol == nil {
			break
		}
		index.Columns = append(index.Columns, indexCol.IndexColumn)
	}
	return nil
}

func (s scanner) scanTableConstraints(c context.Context, catalog string, table *datatug.CollectionInfo, tables datatug.Tables) error {
	constraints, err := s.schemaProvider.GetConstraints(c, catalog, table.Schema, table.Name)
	if err != nil {
		return err
	}
	for {
		constraint, err := constraints.NextConstraint()
		if err != nil {
			return err
		}
		if constraint == nil {
			return nil
		}
		if err = processConstraint(catalog, table, constraint, tables); err != nil {
			return fmt.Errorf("failed to process contraint record: %w", err)
		}
	}
}

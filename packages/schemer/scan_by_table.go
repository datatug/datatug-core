package schemer

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"time"
)

func (s scanner) getTableProps(c context.Context, db *sql.DB, catalog string, table *models.Table) error {
	err := parallel.Run(
		func() (err error) {
			if err = s.scanTableCols(c, db, catalog, table); err != nil {
				return fmt.Errorf("failed to get table columns: %w", err)
			}
			return nil
		},
		func() (err error) {
			if err = s.scanTableIndexes(c, db, catalog, table); err != nil {
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

func (s scanner) scanTableIndexes(c context.Context, db *sql.DB, catalog string, table *models.Table) error {
	indexesReader, err := s.schemaProvider.GetIndexes(c, db, catalog, table.Schema, table.Name)
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
			if err := s.scanIndexColumns(c, db, catalog, table, index.Index); err != nil {
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

func (s scanner) scanIndexColumns(c context.Context, db *sql.DB, catalog string, table *models.Table, index *models.Index) error {
	indexColumnsReader, err := s.schemaProvider.GetIndexColumns(c, db, catalog, table.Schema, table.Name, index.Name)
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

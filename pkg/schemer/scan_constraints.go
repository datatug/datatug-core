package schemer

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s scanner) scanConstraintsInBulk(c context.Context, catalog string, tablesFinder SortedTables) error {
	reader, err := s.schemaProvider.GetConstraints(c, catalog, "", "")
	if err != nil {
		return err
	}
	deadline, isDeadlineSet := c.Deadline()
	for {
		if isDeadlineSet && time.Now().After(deadline) {
			return fmt.Errorf("exceeded deadline")
		}
		constraint, err := reader.NextConstraint()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		table := tablesFinder.SequentialFind(catalog, constraint.SchemaName, constraint.TableName)
		if table == nil {
			return fmt.Errorf("unknown table referenced by constraint [%v]: %v.%v.%v",
				constraint.Name, catalog, constraint.SchemaName, constraint.TableName)
		}
		if err = processConstraint(catalog, table, constraint, tablesFinder.Tables); err != nil {
			return fmt.Errorf("failed to process contraint record: %w", err)
		}
	}
	return nil
}

func processConstraint(catalog string, table *datatug.CollectionInfo, constraint *Constraint, allTables datatug.Tables) error {
	switch constraint.Type {
	case "PRIMARY KEY":
		if table.PrimaryKey == nil {
			table.PrimaryKey = &datatug.UniqueKey{Name: constraint.Name, Columns: []string{constraint.ColumnName}}
		} else {
			table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, constraint.ColumnName)
		}
	case "UNIQUE":
		if len(table.AlternateKeys) > 0 && table.AlternateKeys[len(table.AlternateKeys)-1].Name == constraint.Name {
			i := len(table.AlternateKeys) - 1
			table.AlternateKeys[i].Columns = append(table.AlternateKeys[i].Columns, constraint.ColumnName)
		} else {
			table.AlternateKeys = append(table.AlternateKeys, datatug.UniqueKey{Name: constraint.Name, Columns: []string{constraint.ColumnName}})
		}
	case "FOREIGN KEY":
		if len(table.ForeignKeys) > 0 && table.ForeignKeys[len(table.ForeignKeys)-1].Name == constraint.Name {
			i := len(table.ForeignKeys) - 1
			table.ForeignKeys[i].Columns = append(table.ForeignKeys[i].Columns, constraint.ColumnName)
		} else {
			//refTable := refTableFinder.FindTable(refTableCatalog, refTableSchema, refTableName)
			fk := datatug.ForeignKey{
				Name:     constraint.Name,
				Columns:  []string{constraint.ColumnName},
				RefTable: datatug.NewCollectionKey(datatug.CollectionTypeTable, constraint.RefTableName, constraint.RefTableSchema, constraint.RefTableCatalog, nil),
			}
			fk.MatchOption = constraint.MatchOption
			fk.UpdateRule = constraint.UpdateRule
			fk.DeleteRule = constraint.DeleteRule
			table.ForeignKeys = append(table.ForeignKeys, &fk)

			{ // Update reference table
				refTable := FindTable(allTables, constraint.RefTableCatalog, constraint.RefTableSchema, constraint.RefTableName)
				var refByFk *datatug.RefByForeignKey
				if refTable == nil {
					return fmt.Errorf("reference table not found: %v.%v.%v", constraint.RefTableCatalog, constraint.RefTableSchema, constraint.RefTableName)
				}
				var refByTable *datatug.ReferencedBy
				for _, refByTable = range refTable.ReferencedBy {
					if refByTable.Catalog() == catalog && refByTable.Schema() == constraint.SchemaName && refByTable.Name() == constraint.TableName {
						break
					}
				}
				if refByTable == nil || refByTable.Catalog() != catalog || refByTable.Schema() != constraint.SchemaName || refByTable.Name() != constraint.TableName {
					refByTable = &datatug.ReferencedBy{DBCollectionKey: table.DBCollectionKey, ForeignKeys: make([]*datatug.RefByForeignKey, 0, 1)}
					refTable.ReferencedBy = append(refTable.ReferencedBy, refByTable)
				}
				for _, fk2 := range refByTable.ForeignKeys {
					if fk2.Name == fk.Name {
						refByFk = fk2
						goto fkAddedToRefByTable
					}
				}
				refByFk = &datatug.RefByForeignKey{
					Name:        fk.Name,
					MatchOption: fk.MatchOption,
					UpdateRule:  fk.UpdateRule,
					DeleteRule:  fk.DeleteRule,
				}
				refByTable.ForeignKeys = append(refByTable.ForeignKeys, refByFk)
			fkAddedToRefByTable:
				refByFk.Columns = append(refByFk.Columns, constraint.ColumnName)
			}
		}
	}
	return nil
}

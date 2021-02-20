package schemer

import (
	"context"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"time"
)

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
		if constraint.Constraint == nil {
			break
		}
		table := tablesFinder.SequentialFind(catalog, constraint.SchemaName, constraint.TableName)
		if table == nil {
			return fmt.Errorf("unknown table referenced by constraint [%v]: %v.%v.%v",
				constraint.Name, catalog, constraint.SchemaName, constraint.TableName)
		}
		if err = processConstraint(catalog, table, constraint, tablesFinder); err != nil {
			return fmt.Errorf("failed to process contraint record: %w", err)
		}
	}
	return nil
}

func processConstraint(catalog string, table *models.Table, constraint Constraint, tablesFinder sortedTables) error {
	switch constraint.Type {
	case "PRIMARY KEY":
		if table.PrimaryKey == nil {
			table.PrimaryKey = &models.UniqueKey{Name: constraint.Name, Columns: []string{constraint.ColumnName}}
		} else {
			table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, constraint.ColumnName)
		}
	case "UNIQUE":
		if len(table.UniqueKeys) > 0 && table.UniqueKeys[len(table.UniqueKeys)-1].Name == constraint.ColumnName {
			i := len(table.UniqueKeys) - 1
			table.UniqueKeys[i].Columns = append(table.UniqueKeys[i].Columns, constraint.ColumnName)
		} else {
			table.UniqueKeys = append(table.UniqueKeys, &models.UniqueKey{Name: constraint.Name, Columns: []string{constraint.ColumnName}})
		}
	case "FOREIGN KEY":
		if len(table.ForeignKeys) > 0 && table.ForeignKeys[len(table.ForeignKeys)-1].Name == constraint.Name {
			i := len(table.ForeignKeys) - 1
			table.ForeignKeys[i].Columns = append(table.ForeignKeys[i].Columns, constraint.ColumnName)
		} else {
			//refTable := refTableFinder.FindTable(refTableCatalog, refTableSchema, refTableName)
			fk := models.ForeignKey{
				Name: constraint.Name,
				Columns: []string{
					constraint.ColumnName},
				RefTable: models.TableKey{Catalog: constraint.RefTableCatalog.String, Schema: constraint.RefTableSchema.String, Name: constraint.RefTableName.String},
			}
			if constraint.MatchOption.Valid {
				fk.MatchOption = constraint.MatchOption.String
			}
			if constraint.UpdateRule.Valid {
				fk.UpdateRule = constraint.UpdateRule.String
			}
			if constraint.DeleteRule.Valid {
				fk.DeleteRule = constraint.DeleteRule.String
			}
			table.ForeignKeys = append(table.ForeignKeys, &fk)

			{ // Update reference table
				refTable := findTable(tablesFinder.tables, constraint.RefTableCatalog.String, constraint.RefTableSchema.String, constraint.RefTableName.String)
				var refByFk *models.RefByForeignKey
				if refTable == nil {
					return fmt.Errorf("reference table not found: %v.%v.%v", constraint.RefTableCatalog.String, constraint.RefTableSchema.String, constraint.RefTableName.String)
				}
				var refByTable *models.TableReferencedBy
				for _, refByTable = range refTable.ReferencedBy {
					if refByTable.Catalog == catalog && refByTable.Schema == constraint.SchemaName && refByTable.Name == constraint.TableName {
						break
					}
				}
				if refByTable == nil || refByTable.Catalog != catalog || refByTable.Schema != constraint.SchemaName || refByTable.Name != constraint.TableName {
					refByTable = &models.TableReferencedBy{TableKey: table.TableKey, ForeignKeys: make([]*models.RefByForeignKey, 0, 1)}
					refTable.ReferencedBy = append(refTable.ReferencedBy, refByTable)
				}
				for _, fk2 := range refByTable.ForeignKeys {
					if fk2.Name == fk.Name {
						refByFk = fk2
						goto fkAddedToRefByTable
					}
				}
				refByFk = &models.RefByForeignKey{
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

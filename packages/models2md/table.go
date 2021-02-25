package models2md

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"io"
	"net/url"
	"strings"
	"time"
)

// EncodeTable encodes table summary to markdown file format
func (encoder) EncodeTable(w io.Writer, catalog string, table *models.Table, dbServer models.ProjDbServer) error {

	recordsCount := ""
	if table.RecordsCount != nil {
		recordsCount = fmt.Sprintf("Number of records at time of last scan: %v", *table.RecordsCount)
	}
	var primaryKey string
	if table.PrimaryKey == nil {
		primaryKey = "*None*"
	} else {
		pkCols := make([]string, len(table.PrimaryKey.Columns))
		for i, pkCol := range table.PrimaryKey.Columns {
			pkCols[i] = fmt.Sprintf("**%v**", pkCol)
		}
		primaryKey = fmt.Sprintf("`%v` (%v)", table.PrimaryKey.Name, strings.Join(pkCols, ", "))
	}

	var foreignKeys string
	if len(table.ForeignKeys) == 0 {
		foreignKeys = "*None*"
	} else {
		fks := make([]string, len(table.ForeignKeys))
		for i, fk := range table.ForeignKeys {
			joinSQL := strings.TrimSpace(fmt.Sprintf(`
USE %v
SELECT
	*
FROM %v.%v
%v JOIN %v.%v ON`, catalog, table.Schema, table.Name, "%v", fk.RefTable.Schema, fk.RefTable.Name))

			fkRefTable := dbServer.DbCatalogs.GetTable(catalog, fk.RefTable.Schema, fk.RefTable.Name)
			if fkRefTable == nil {
				return fmt.Errorf("table %v.%v is referencing via %v to unkown table %v.%v", table.Schema, table.Name, fk.Name, fk.RefTable.Schema, fk.RefTable.Name)
			}
			for i, fkCol := range fk.Columns {
				if i > 0 {
					joinSQL += " AND"
				}
				if fkRefTable.Name != table.Name {
					joinSQL += fmt.Sprintf(" %v.%v = %v.%v", fkRefTable.Name, fkRefTable.PrimaryKey.Columns[i], table.Name, fkCol)
				} else {
					joinSQL += fmt.Sprintf(" %v.%v.%v = %v.%v.%v", fkRefTable.Schema, fkRefTable.Name, fkRefTable.PrimaryKey.Columns[i], table.Schema, table.Name, fkCol)
				}
			}

			joinMD := func(kind string) string {
				text := url.QueryEscape(fmt.Sprintf(joinSQL, kind))
				return fmt.Sprintf("<a href='https://datatug.app/query#text=%v' target='_blank'>%v</a>", text, kind)
			}
			joins := []string{
				joinMD("LEFT"),
				joinMD("INNER"),
				joinMD("RIGHT"),
			}
			fks[i] = fmt.Sprintf("- `%v` (%v) ⇒ [%v](../../../%v).[%v](../../../%v/tables/%v)",
				fk.Name,
				fmt.Sprintf("**%v**", strings.Join(fk.Columns, "**, **")),
				fk.RefTable.Schema, fk.RefTable.Schema, fk.RefTable.Name,
				fk.RefTable.Schema, fk.RefTable.Name,
			) + "\n  <br>&nbsp;&nbsp;SQL *to* JOIN: " + strings.Join(joins, " | ")
		}
		//<br>&nbsp;&nbsp;&nbsp;&nbsp;SQL to JOIN: [LEFT](left) | [INNER](inner) | [RIGHT](right)
		foreignKeys = strings.Join(fks, "\n")
	}

	var allReferencedBy string
	var referencedBy string
	if len(table.ReferencedBy) == 0 {
		referencedBy = "*None*"
	} else {
		refBys := make([]string, 0, len(table.ReferencedBy))
		err := walkReferencedBy(table, 0, nil, func(parent *models.Table, refBy *models.TableReferencedBy, level int) error {
			return writeRefByToMarkDownListTree(catalog, refBys, parent, refBy, level)
		})
		if err != nil {
			return fmt.Errorf("failed to process referencedBy: %w", err)
		}
		referencedBy = strings.Join(refBys, "\n")
	}

	columns := make([]string, len(table.Columns))
	for i, c := range table.Columns {
		columns[i] = fmt.Sprintf("- `%v` %v", c.Name, c.DbType)

		indexes := make([]string, 0, len(table.Indexes))
		{ // Write column indexes
			for _, index := range table.Indexes {
				for _, indexCol := range index.Columns {
					if indexCol.Name == c.Name {
						indexes = append(indexes, fmt.Sprintf("`%v`", index.Name))
						break
					}
				}
			}
			if len(indexes) > 0 {
				columns[i] += "\n  - *Indexes*: " + strings.Join(indexes, ", ")
			}
		}
		{ // Write column FK participation
			foreignKeys := make([]string, 0, len(table.ForeignKeys))
			for _, fk := range table.ForeignKeys {
				for _, fkCol := range fk.Columns {
					if fkCol == c.Name {
						foreignKeys = append(foreignKeys, fmt.Sprintf("`%v`", fk.Name))
						break
					}
				}
			}
			switch len(foreignKeys) {
			case 0:
				// Pass
			case 1:
				columns[i] += "\n  - *Foreign key*: " + foreignKeys[0]
			default:
				columns[i] += "\n  - *Foreign keys*: " + strings.Join(foreignKeys, ", ")
			}
			if len(foreignKeys) > 0 && len(indexes) == 0 {
				columns[i] += "\n  - **WARNING** - participates in a foreign key but is not part of any index"
			}
		}
	}

	indexes := make([]string, len(table.Indexes))
	for i, index := range table.Indexes {
		cols := make([]string, len(index.Columns))
		for k, col := range index.Columns {
			if col.IsDescending {
				cols[k] = col.Name + " DESC"
			} else {
				cols[k] = col.Name
			}
		}
		indexes[i] = fmt.Sprintf("- %v (%v) `%v`", index.Name, strings.Join(cols, ", "), index.Type)
		indexProps := make([]string, 0)
		if index.IsPrimaryKey {
			indexProps = append(indexProps, "`PRIMARY KEY`")
		} else if index.IsUnique {
			indexProps = append(indexProps, "`UNIQUE`")
		}
		if index.IsUniqueConstraint {
			indexProps = append(indexProps, "`UNIQUE CONSTRAINT`")
		}
		if index.IsHash {
			indexProps = append(indexProps, "`HASH`")
		}
		if index.IsXml {
			indexProps = append(indexProps, "`XML`")
		}
		if index.IsColumnStore {
			indexProps = append(indexProps, "`COLUMN_STORE`")
		}
		if len(indexProps) > 0 {
			indexes[i] += " - " + strings.Join(indexProps, ", ")
		}
	}

	var indexesStr string
	if len(indexes) > 0 {
		indexesStr = strings.Join(indexes, "\n")
	} else {
		indexesStr = "*No indexes*"
	}

	now := time.Now()
	_, err := fmt.Fprintf(w, `
# Table: [%v](..).%v
%v

[📝 Edit query](https://datatug.app/edit-query) *or* [▶️ Execute query](https://datatug.app/execute-query) 
%v
USE %v;
SELECT * FROM %v.%v;
%v

## Primary key
%v

## Foreign keys
%v

## Columns
%v

## Indexes
%v

## Referenced by

Direct references to this object:
%v

All references to this object:
%v

> Generated by free [DataTug](https://datatug.app) [agent](https://github.com/datatug/datatug) on %v
`,
		table.Schema,
		table.Name,
		recordsCount,
		"```", catalog, table.Schema, table.Name, "```",
		primaryKey,
		foreignKeys,
		strings.Join(columns, "\n"),
		referencedBy,
		allReferencedBy,
		indexesStr,
		now.Format("January 02, 2006 at 15:04 pm"),
	)

	return err
}

func walkReferencedBy(table *models.Table, level int, processed []string, process func(parent *models.Table, refBy *models.TableReferencedBy, level int) error) error {
	level++
	for _, refBy := range table.ReferencedBy {
		process(table, refBy, level)
	}
	return nil
}

func writeRefByToMarkDownListTree(catalog string, lines []string, parent *models.Table, refBy *models.TableReferencedBy, level int) error {
	joinSQL := strings.TrimSpace(fmt.Sprintf(`
USE %v
SELECT
	*
FROM %v.%v
%v JOIN %v.%v ON`, catalog, parent.Schema, parent.Name, "%v", refBy.Schema, refBy.Name))

	s := make([]string, 1, 1+len(parent.PrimaryKey.Columns)*len(refBy.ForeignKeys))
	s[0] = fmt.Sprintf("- [%v](../../../%v).[%v](../../../%v/tables/%v)", refBy.Schema, refBy.Schema, refBy.Name, refBy.Schema, refBy.Name)
	for _, fk := range refBy.ForeignKeys {

		for i, fkCol := range fk.Columns {
			if i > 0 {
				joinSQL += " AND"
			}
			if refBy.Name != parent.Name {
				joinSQL += fmt.Sprintf(" %v.%v = %v.%v", refBy.Name, fkCol, parent.Name, parent.PrimaryKey.Columns[i])
			} else {
				joinSQL += fmt.Sprintf(" %v.%v.%v = %v.%v.%v", refBy.Schema, refBy.Name, fkCol, parent.Schema, parent.Name, parent.PrimaryKey.Columns[i])
			}
		}

		joinMD := func(kind string) string {
			text := url.QueryEscape(fmt.Sprintf(joinSQL, kind))
			return fmt.Sprintf("<a href='https://datatug.app/query#text=%v' target='_blank'>%v</a>", text, kind)
		}
		joins := []string{
			joinMD("LEFT"),
			joinMD("INNER"),
			joinMD("RIGHT"),
		}
		s = append(s, fmt.Sprintf("  - `%v`\n    <br>&nbsp;&nbsp;by columns: `%v` &mdash;", fk.Name, strings.Join(fk.Columns, "`, `"))+
			"\n    <small>JOIN:\n"+strings.Join(joins, " |\n    ")+"\n    </small>")
	}
	lines = append(lines, strings.Join(s, "\n"))
	return nil
}

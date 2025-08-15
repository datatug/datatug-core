package models2md

import (
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"net/url"
	"regexp"
	"strings"
)

var reUnquoted = regexp.MustCompile(`\w+`)

var reUpperCase = regexp.MustCompile("[A-Z]")

// TableToReadme encodes table summary to markdown file format
func getTableData(repository *models.ProjectRepository, catalog string, table *models.Table, dbServer models.ProjDbServer) (data map[string]interface{}, err error) {

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

			fkRefTable := dbServer.Catalogs.GetTable(catalog, fk.RefTable.Schema, fk.RefTable.Name)
			if fkRefTable == nil {
				return nil, fmt.Errorf("table %v.%v is referencing via %v to unknown table %v.%v", table.Schema, table.Name, fk.Name, fk.RefTable.Schema, fk.RefTable.Name)
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

	var repoID, projectID string
	if repository != nil {
		repoURL, err := url.Parse(repository.WebURL)
		if err != nil {
			return nil, fmt.Errorf("project repository has invalid  (%v): %w", repository.WebURL, err)
		}
		repoID = repoURL.Host
		projectID = repository.ProjectID
	}

	var referencedBy string
	if len(table.ReferencedBy) == 0 {
		referencedBy = "*None*"
	} else {
		refBys := make([]string, 0, len(table.ReferencedBy))
		walker := refByWalker{
			catalog:   catalog,
			dbServer:  dbServer,
			processed: make(map[string]*models.Table),
			process: func(parent *models.Table, refBy *models.TableReferencedBy, level, index int) error {
				line, err := writeRefByToMarkDownListTree(repoID, projectID, dbServer.ID, catalog, parent, refBy, level, index)
				if err != nil {
					return err
				}
				refBys = append(refBys, line)
				return nil
			}}
		err := walker.walkReferencedBy(table, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to process referencedBy: %w", err)
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
		if index.IsXML {
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

	var openInDatatugApp string
	if repoID != "" && projectID != "" {
		schema := table.Schema
		if !reUnquoted.MatchString(schema) {
			schema = fmt.Sprintf("[%v]", schema)
		}
		name := table.Name
		if !reUnquoted.MatchString(name) {
			name = fmt.Sprintf("[%v]", name)
		}
		sql := fmt.Sprintf("SELECT\n\t*\nFROM %v.%v", schema, name)
		if len(name) > 5 {
			alias := reUpperCase.FindAllString(name, -1)
			if len(alias) <= 4 && len(name)-len(alias) > 3 {
				sql += " AS " + strings.ToLower(strings.Join(alias, ""))
			}
		}
		link := fmt.Sprintf("https://datatug.app/pwa/repo/%v/project/%v/query?server=%v&catalog=%v#text=%v",
			url.PathEscape(repoID),
			url.PathEscape(projectID),
			url.QueryEscape(dbServer.ID),
			url.QueryEscape(catalog),
			url.QueryEscape(sql))
		openInDatatugApp = fmt.Sprintf("[Edit & run in **DataTug.app**](%v)\n- all the data at your finger tips.", link)
	}

	data = map[string]interface{}{
		"table":            table,
		"catalog":          catalog,
		recordsCount:       recordsCount,
		"openInDatatugApp": openInDatatugApp,
		"primaryKey":       primaryKey,
		"foreignKeys":      foreignKeys,
		"columns":          strings.Join(columns, "\n"),
		"indexes":          indexesStr,
		"referencedBy":     referencedBy,
	}

	return data, nil
}

type refByWalker struct {
	catalog   string
	dbServer  models.ProjDbServer
	processed map[string]*models.Table
	process   func(parent *models.Table, refBy *models.TableReferencedBy, level, index int) error
}

func (refByWalker) getTableID(schema, name string) string {
	return fmt.Sprintf("[%v].[%v]", schema, name)
}

func (walker *refByWalker) walkReferencedBy(table *models.Table, level int) error {
	level++
	walker.processed[walker.getTableID(table.Schema, table.Name)] = table
	for i, refBy := range table.ReferencedBy {
		refByID := walker.getTableID(refBy.Schema, refBy.Name)
		if _, ok := walker.processed[refByID]; ok {
			continue
		}
		if err := walker.process(table, refBy, level, i); err != nil {
			return err
		}
		referringTable := walker.dbServer.Catalogs.GetTable(walker.catalog, refBy.Schema, refBy.Name)
		if referringTable == nil {
			return fmt.Errorf("catalog %v has table [%v.%v] that is referenced by unknown table [%v.%v]",
				walker.catalog, table.Schema, table.Name, refBy.Schema, refBy.Name)
		}
		if len(referringTable.ReferencedBy) > 0 {
			if err := walker.walkReferencedBy(referringTable, level); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeRefByToMarkDownListTree(repoID, projectID, server, catalog string, parent *models.Table, refBy *models.TableReferencedBy, level, index int) (string, error) {
	joinSQL := strings.TrimSpace(fmt.Sprintf(`
USE %v
SELECT
	*
FROM %v.%v
%v JOIN %v.%v ON`, catalog, parent.Schema, parent.Name, "%v", refBy.Schema, refBy.Name))

	const singleIndent = "  "
	indent := strings.Repeat(singleIndent, (level-1)*len(singleIndent))

	s := make([]string, 1)

	if level > 1 {
		if index == 0 {
			s = append(s, indent[:len(indent)-len(singleIndent)]+"- Referenced by:\n")
		}
		//indent += singleIndent
	}

	s = append(s, fmt.Sprintf(indent+"- [%v](../../../%v).[%v](../../../%v/tables/%v)", refBy.Schema, refBy.Schema, refBy.Name, refBy.Schema, refBy.Name))
	fkIndent := indent + singleIndent
	const itemTextPadding = "  "
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
			queryPart := fmt.Sprintf("query?server=%v&catalog=%v#text=%v", server, catalog, text)
			if repoID != "" && projectID != "" {
				return fmt.Sprintf("<a href='https://datatug.app/pwa/repo/%v/project/%v/%v' target='_blank'>%v</a>", repoID, projectID, queryPart, kind)
			}
			return fmt.Sprintf("<a href='https://datatug.app/pwa/%v' target='_blank'>%v</a>", queryPart, kind)
		}
		joins := []string{
			joinMD("LEFT"),
			joinMD("INNER"),
			joinMD("RIGHT"),
		}
		s = append(s, fmt.Sprintf(fkIndent+"- `%v`\n"+
			fkIndent+itemTextPadding+"<br>by columns: `%v` &mdash;", fk.Name, strings.Join(fk.Columns, "`, `"))+"\n"+
			fkIndent+itemTextPadding+"<small>JOIN:\n"+
			fkIndent+itemTextPadding+strings.Join(joins, " |\n"+fkIndent+itemTextPadding)+"\n"+
			fkIndent+itemTextPadding+"</small>",
		)
	}
	return strings.Join(s, "\n"), nil
}

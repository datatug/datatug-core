package schemer

import (
	"database/sql"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"strings"
)

// InformationSchema provides API to retrieve information about a database
type InformationSchema struct {
	db     *sql.DB
	server models.ServerReference
}

// NewInformationSchema creates new InformationSchema
func NewInformationSchema(server models.ServerReference, db *sql.DB) InformationSchema {
	return InformationSchema{server: server, db: db}
}

// GetDatabase returns complete information about a database
func (s InformationSchema) GetDatabase(name string) (database *models.DbCatalog, err error) {
	database = new(models.DbCatalog)
	database.ID = name
	var tables []*models.Table
	if tables, err = s.getTables(name); err != nil {
		return database, fmt.Errorf("failed to retrive tables metadata: %w", err)
	}
	for _, t := range tables {
		schema := database.Schemas.GetByID(t.Schema)
		if schema == nil {
			schema = new(models.DbSchema)
			schema.ID = t.Schema
			database.Schemas = append(database.Schemas, schema)
		}
		switch t.DbType {
		case "BASE TABLE":
			schema.Tables = append(schema.Tables, t)
		case "VIEW":
			schema.Views = append(schema.Views, t)
		default:
			err = fmt.Errorf("object [%v] has unknown DB type: %v", t.Name, t.DbType)
			return
		}
	}
	log.Printf("%v schemas.", len(database.Schemas))
	err = parallel.Run(
		func() error {
			if err = s.getColumns(name, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive columns metadata: %w", err)
			}
			return nil
		},
		func() error {
			if err = s.getConstraints(name, sortedTables{tables: tables}); err != nil {
				return fmt.Errorf("failed to retrive constraints metadata: %w", err)
			}
			return nil
		},
	)
	log.Println("GetDatabase completed")
	return
}

func (s InformationSchema) getTables(catalog string) (tables []*models.Table, err error) {
	var rows *sql.Rows
	//goland:noinspection SqlNoDataSourceInspection
	rows, err = s.db.Query(`SELECT
       TABLE_SCHEMA,
       TABLE_NAME,
       TABLE_TYPE
FROM INFORMATION_SCHEMA.TABLES
ORDER BY TABLE_SCHEMA, TABLE_NAME`)
	if err != nil {
		return nil, fmt.Errorf("failed to query INFORMATION_SCHEMA.TABLES: %w", err)
	}
	tables = make([]*models.Table, 0)
	for rows.Next() {
		var table models.Table
		if err = rows.Scan(&table.Schema, &table.Name, &table.DbType); err != nil {
			return nil, fmt.Errorf("failed to scan table row into Table struct: %w", err)
		}
		table.Catalog = catalog
		tables = append(tables, &table)
	}
	log.Printf("Retrieved %v tables.", len(tables))
	//for i, t := range tables {
	//	log.Printf("\t%v: %v: %+v", i, t.ID, t)
	//}
	return tables, err
}

func (s InformationSchema) getConstraints(catalog string, tablesFinder sortedTables) (err error) {
	log.Println("Getting constraints...")
	var rows *sql.Rows
	//goland:noinspection SqlNoDataSourceInspection
	rows, err = s.db.Query(`SELECT
	tc.TABLE_SCHEMA, tc.TABLE_NAME,
    tc.CONSTRAINT_TYPE, kcu.CONSTRAINT_NAME,
    kcu.COLUMN_NAME,-- kcu.ORDINAL_POSITION,
	rc.UNIQUE_CONSTRAINT_CATALOG, rc.UNIQUE_CONSTRAINT_SCHEMA, rc.UNIQUE_CONSTRAINT_NAME,
    rc.MATCH_OPTION, rc.UPDATE_RULE, rc.DELETE_RULE,
	kcu2.TABLE_CATALOG AS REF_TABLE_CATALOG, kcu2.TABLE_SCHEMA AS REF_TABLE_SCHEMA, kcu2.TABLE_NAME AS REF_TABLE_NAME, kcu2.COLUMN_NAME AS REF_COL_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS kcu
INNER JOIN INFORMATION_SCHEMA.TABLE_CONSTRAINTS AS tc ON tc.CONSTRAINT_CATALOG = kcu.CONSTRAINT_CATALOG AND tc.CONSTRAINT_SCHEMA = kcu.CONSTRAINT_SCHEMA AND tc.CONSTRAINT_NAME = kcu.CONSTRAINT_NAME
LEFT JOIN INFORMATION_SCHEMA.REFERENTIAL_CONSTRAINTS AS rc ON tc.CONSTRAINT_TYPE = 'FOREIGN KEY' AND rc.CONSTRAINT_CATALOG = tc.CONSTRAINT_CATALOG AND rc.CONSTRAINT_SCHEMA = tc.CONSTRAINT_SCHEMA AND rc.CONSTRAINT_NAME = tc.CONSTRAINT_NAME
LEFT JOIN INFORMATION_SCHEMA.KEY_COLUMN_USAGE AS kcu2 ON kcu2.CONSTRAINT_CATALOG = rc.UNIQUE_CONSTRAINT_CATALOG AND kcu2.CONSTRAINT_SCHEMA = rc.UNIQUE_CONSTRAINT_SCHEMA AND kcu2.CONSTRAINT_NAME = rc.UNIQUE_CONSTRAINT_NAME AND kcu2.ORDINAL_POSITION = kcu.ORDINAL_POSITION
ORDER BY tc.TABLE_SCHEMA, tc.TABLE_NAME, tc.CONSTRAINT_TYPE, kcu.CONSTRAINT_NAME, kcu.ORDINAL_POSITION`)
	if err != nil {
		return err
	}

	var (
		tSchema, tName                                                        string
		constraintType, constraintName                                        string
		columnName                                                            string
		uniqueConstraintCatalog, uniqueConstraintSchema, uniqueConstraintName sql.NullString
		matchOption, updateRule, deleteRule                                   sql.NullString
		refTableCatalog, refTableSchema, refTableName, refColName             sql.NullString
	)

	for rows.Next() {
		if err = rows.Scan(
			&tSchema, &tName,
			&constraintType, &constraintName,
			&columnName,
			&uniqueConstraintCatalog, &uniqueConstraintSchema, &uniqueConstraintName,
			&matchOption, &updateRule, &deleteRule,
			&refTableCatalog, &refTableSchema, &refTableName, &refColName,
		); err != nil {
			return fmt.Errorf("failed to scan constaints record: %w", err)
		}
		if table := tablesFinder.SequentialFind(catalog, tSchema, tName); table != nil {
			switch constraintType {
			case "PRIMARY KEY":
				if table.PrimaryKey == nil {
					table.PrimaryKey = &models.UniqueKey{Name: constraintName, Columns: []string{columnName}}
				} else {
					table.PrimaryKey.Columns = append(table.PrimaryKey.Columns, columnName)
				}
			case "UNIQUE":
				if len(table.UniqueKeys) > 0 && table.UniqueKeys[len(table.UniqueKeys)-1].Name == constraintName {
					i := len(table.UniqueKeys) - 1
					table.UniqueKeys[i].Columns = append(table.UniqueKeys[i].Columns, columnName)
				} else {
					table.UniqueKeys = append(table.UniqueKeys, &models.UniqueKey{Name: constraintName, Columns: []string{columnName}})
				}
			case "FOREIGN KEY":
				if len(table.ForeignKeys) > 0 && table.ForeignKeys[len(table.ForeignKeys)-1].Name == constraintName {
					i := len(table.ForeignKeys) - 1
					table.ForeignKeys[i].Columns = append(table.ForeignKeys[i].Columns, columnName)
				} else {
					//refTable := refTableFinder.FindTable(refTableCatalog, refTableSchema, refTableName)
					fk := models.ForeignKey{
						Name: constraintName,
						Columns: []string{
							columnName},
						RefTable: models.TableKey{Catalog: refTableCatalog.String, Schema: refTableSchema.String, Name: refTableName.String},
					}
					if matchOption.Valid {
						fk.MatchOption = matchOption.String
					}
					if updateRule.Valid {
						fk.UpdateRule = updateRule.String
					}
					if deleteRule.Valid {
						fk.DeleteRule = deleteRule.String
					}
					table.ForeignKeys = append(table.ForeignKeys, &fk)

					{ // Update reference table
						refTable := findTable(tablesFinder.tables, refTableCatalog.String, refTableSchema.String, refTableName.String)
						var refByFk *models.RefByForeignKey
						if refTable == nil {
							return fmt.Errorf("reference table not found: %v.%v.%v", refTableCatalog.String, refTableSchema.String, refTableName.String)
						}
						var refByTable *models.TableReferencedBy
						for _, refByTable = range refTable.ReferencedBy {
							if refByTable.Catalog == catalog && refByTable.Schema == tSchema && refByTable.Name == tName {
								break
							}
						}
						if refByTable == nil || refByTable.Catalog != catalog || refByTable.Schema != tSchema || refByTable.Name != tName {
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
						refByFk.Columns = append(refByFk.Columns, columnName)
					}
				}
			}
		} else {
			log.Printf("Table not found: %v.%v.%v, table: %+v", catalog, tSchema, tName, table)
			continue
		}
	}

	return err
}

type sortedTables struct {
	tables []*models.Table
	i      int
}

func (sorted *sortedTables) Reset() {
	sorted.i = 0
}

// SequentialFind will work if calls to it are issued in lexical order
func (sorted *sortedTables) SequentialFind(catalog, schema, name string) *models.Table {
	for ; sorted.i < len(sorted.tables); sorted.i++ {
		t := sorted.tables[sorted.i]
		if t.Name == name && t.Schema == schema && t.Catalog == catalog {
			return t
		}
	}
	return nil
}

type sortedIndexes struct {
	indexes []*Index
	i       int
}

func (sorted *sortedIndexes) Reset() {
	sorted.i = 0
}

// SequentialFind will work if calls to it are issued in lexical order
func (sorted *sortedIndexes) SequentialFind(schema, table, name string) *Index {
	for ; sorted.i < len(sorted.indexes); sorted.i++ {
		index := sorted.indexes[sorted.i]
		if index.Name == name && index.TableName == table && index.SchemaName == schema {
			return index
		}
	}
	return nil
}

// FullFind can be called in any order and always do a full table scan
func findTable(tables models.Tables, catalog, schema, name string) *models.Table {
	normalize := strings.ToLower
	catalog = normalize(catalog)
	schema = normalize(schema)
	name = normalize(name)
	for _, t := range tables {
		if normalize(t.Name) == name && normalize(t.Schema) == schema && normalize(t.Catalog) == catalog {
			return t
		}
	}
	return nil
}

func (s InformationSchema) getColumns(catalog string, tablesFinder sortedTables) (err error) {
	log.Println("Getting columns...")
	var rows *sql.Rows
	//goland:noinspection SqlNoDataSourceInspection
	rows, err = s.db.Query(`SELECT
    TABLE_SCHEMA,
    TABLE_NAME,
    COLUMN_NAME,
    ORDINAL_POSITION,
    COLUMN_DEFAULT,
    IS_NULLABLE,
    DATA_TYPE,
    CHARACTER_MAXIMUM_LENGTH,
    CHARACTER_OCTET_LENGTH,
	CHARACTER_SET_CATALOG,
	CHARACTER_SET_SCHEMA,
    CHARACTER_SET_NAME,
	COLLATION_CATALOG,
	COLLATION_SCHEMA,
    COLLATION_NAME
FROM INFORMATION_SCHEMA.COLUMNS ORDER BY TABLE_SCHEMA, TABLE_NAME, ORDINAL_POSITION`)
	if err != nil {
		return fmt.Errorf("failed to query INFORMATION_SCHEMA.COLUMNS: %w", err)
	}
	var isNullable string
	var charSetCatalog, charSetSchema, charSetName sql.NullString
	var collationCatalog, collationSchema, collationName sql.NullString
	i := 0
	for rows.Next() {
		i++
		c := new(models.TableColumn)
		var tSchema, tName string
		if err = rows.Scan(
			&tSchema,
			&tName,
			&c.Name,
			&c.OrdinalPosition,
			&c.Default,
			&isNullable,
			&c.DbType,
			&c.CharMaxLength,
			&c.CharOctetLength,
			&charSetCatalog,
			&charSetSchema,
			&charSetName,
			&collationCatalog,
			&collationSchema,
			&collationName,
		); err != nil {
			return fmt.Errorf("failed to scan INFORMATION_SCHEMA.COLUMNS row into TableColumn struct: %w", err)
		}
		switch isNullable {
		case "YES":
			c.IsNullable = true
		case "NO":
			c.IsNullable = false
		default:
			err = fmt.Errorf("unknown value for IS_NULLABLE: %v", isNullable)
			return
		}
		if charSetName.Valid && charSetName.String != "" {
			c.CharacterSet = &models.CharacterSet{Name: charSetName.String}
			if charSetSchema.Valid {
				c.CharacterSet.Schema = charSetSchema.String
			}
			if charSetCatalog.Valid {
				c.CharacterSet.Catalog = charSetCatalog.String
			}
		}
		if collationName.Valid && collationName.String != "" {
			c.Collation = &models.Collation{Name: collationName.String}
			//if collationSchema.Valid {
			//	c.Collation.Schema = collationSchema.String
			//}
			//if collationCatalog.Valid {
			//	c.Collation.Catalog = collationCatalog.String
			//}
		}
		/*
			if table == nil || tName != table.ID || tSchema != table.Schema || tCatalog != table.Catalog {
				for _, t := range tables {
					if t.ID == tName && t.Schema == tSchema && t.Catalog == tCatalog {
						//log.Printf("Found table: %+v", t)
						table = t
						break
					}
				}
			}
			if table == nil || table.ID != tName || table.Schema != tSchema || table.Catalog != tCatalog {
			}
		*/
		if table := tablesFinder.SequentialFind(catalog, tSchema, tName); table != nil {
			table.Columns = append(table.Columns, c)
		} else {
			log.Printf("Table not found: %v.%v.%v, table: %+v", catalog, tSchema, tName, table)
			continue
		}
	}
	fmt.Println("Processed columns:", i)
	return err
}

func (s InformationSchema) getIndexes(catalog string, tablesFinder sortedTables) (err error) {
	log.Println("Getting indexes...")
	var rows *sql.Rows
	//goland:noinspection SqlNoDataSourceInspection
	rows, err = s.db.Query(`SELECT 
    SCHEMA_NAME(o.schema_id) AS schema_name,
	o.name AS object_name,
    CASE WHEN o.type = 'U' THEN 'Table'
        WHEN o.type = 'V' THEN 'View'
		ELSE o.type
        END AS object_type,
	--i.name as index_name,
	--column_names,
	/*
    case when i.type = 1 then 'IsClustered index'
        when i.type = 2 then 'Nonclustered unique index'
        when i.type = 3 then 'XML index'
        when i.type = 4 then 'Spatial index'
        when i.type = 5 then 'IsClustered columnstore index'
        when i.type = 6 then 'Nonclustered columnstore index'
        when i.type = 7 then 'Nonclustered hash index'
        end as type_description,*/
    i.*
FROM sys.indexes AS i
INNER JOIN sys.objects o ON o.object_id = i.object_id
WHERE o.is_ms_shipped <> 1 --and index_id > 0
ORDER BY SCHEMA_NAME(o.schema_id) + '.' + o.name, i.name;`)
	if err != nil {
		return fmt.Errorf("failed to query INFORMATION_SCHEMA.COLUMNS: %w", err)
	}
	var isNullable string
	var charSetCatalog, charSetSchema, charSetName sql.NullString
	var collationCatalog, collationSchema, collationName sql.NullString
	i := 0
	for rows.Next() {
		i++
		c := new(models.TableColumn)
		var tSchema, tName string
		if err = rows.Scan(
			&tSchema,
			&tName,
			&c.Name,
			&c.OrdinalPosition,
			&c.Default,
			&isNullable,
			&c.DbType,
			&c.CharMaxLength,
			&c.CharOctetLength,
			&charSetCatalog,
			&charSetSchema,
			&charSetName,
			&collationCatalog,
			&collationSchema,
			&collationName,
		); err != nil {
			return fmt.Errorf("failed to scan INFORMATION_SCHEMA.COLUMNS row into TableColumn struct: %w", err)
		}
		switch isNullable {
		case "YES":
			c.IsNullable = true
		case "NO":
			c.IsNullable = false
		default:
			err = fmt.Errorf("unknown value for IS_NULLABLE: %v", isNullable)
			return
		}
		if charSetName.Valid && charSetName.String != "" {
			c.CharacterSet = &models.CharacterSet{Name: charSetName.String}
			if charSetSchema.Valid {
				c.CharacterSet.Schema = charSetSchema.String
			}
			if charSetCatalog.Valid {
				c.CharacterSet.Catalog = charSetCatalog.String
			}
		}
		if collationName.Valid && collationName.String != "" {
			c.Collation = &models.Collation{Name: collationName.String}
			//if collationSchema.Valid {
			//	c.Collation.Schema = collationSchema.String
			//}
			//if collationCatalog.Valid {
			//	c.Collation.Catalog = collationCatalog.String
			//}
		}
		/*
			if table == nil || tName != table.ID || tSchema != table.Schema || tCatalog != table.Catalog {
				for _, t := range tables {
					if t.ID == tName && t.Schema == tSchema && t.Catalog == tCatalog {
						//log.Printf("Found table: %+v", t)
						table = t
						break
					}
				}
			}
			if table == nil || table.ID != tName || table.Schema != tSchema || table.Catalog != tCatalog {
			}
		*/
		if table := tablesFinder.SequentialFind(catalog, tSchema, tName); table != nil {
			table.Columns = append(table.Columns, c)
		} else {
			log.Printf("Table not found: %v.%v.%v, table: %+v", catalog, tSchema, tName, table)
			continue
		}
	}
	fmt.Println("Processed columns:", i)
	return err
}

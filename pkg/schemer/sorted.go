package schemer

import (
	"strings"

	"github.com/datatug/datatug-core/pkg/models"
)

type SortedTables struct {
	Tables []*models.CollectionInfo
	i      int
}

func (sorted *SortedTables) Reset() {
	sorted.i = 0
}

// SequentialFind will work if calls to it are issued in lexical order
func (sorted *SortedTables) SequentialFind(catalog, schema, name string) *models.CollectionInfo {
	for ; sorted.i < len(sorted.Tables); sorted.i++ {
		t := sorted.Tables[sorted.i]
		if t.Name == name && t.Schema == schema && t.Catalog == catalog {
			return t
		}
	}
	return nil
}

type SortedIndexes struct {
	indexes []*Index
	i       int
}

func (sorted *SortedIndexes) Reset() {
	sorted.i = 0
}

// SequentialFind will work if calls to it are issued in lexical order
func (sorted *SortedIndexes) SequentialFind(schema, table, name string) *Index {
	for ; sorted.i < len(sorted.indexes); sorted.i++ {
		index := sorted.indexes[sorted.i]
		if index.Name == name && index.TableName == table && index.SchemaName == schema {
			return index
		}
	}
	return nil
}

// FullFind can be called in any order and always do a full table scan
func FindTable(tables models.Tables, catalog, schema, name string) *models.CollectionInfo {
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

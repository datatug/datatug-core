package filestore

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// The indexes of a DB model are stored in one file beside the model's own
// file, "<model>.indexes.json". The format is a public contract, specified in
// spec/features/model-indexes-file; this file follows that page. It is built
// as the foreign keys file is (model_foreign_keys_file.go) and shares its
// location rule, its version check and its errors.

const (
	// indexesFileVersion is the only version of the file this release reads
	// and writes.
	indexesFileVersion = 1
	// maxIndexesFileSize is the biggest file a reader reads.
	maxIndexesFileSize int64 = 64 << 20
)

// indexesFile is the content of "<model>.indexes.json". The members are
// written in the order they are declared in.
type indexesFile struct {
	Version int               `json:"version"`
	Indexes []indexesFileItem `json:"indexes"`
}

// indexesFileRead is what a reader decodes the file into. The list is a
// pointer so that an absent "indexes" and a null one can be told from an
// empty list, which is the only way the format says "no indexes".
type indexesFileRead struct {
	Indexes *[]indexesFileItem `json:"indexes"`
}

// indexesFileItem is one index and the table it is on.
type indexesFileItem struct {
	Name    string               `json:"name"`
	Table   foreignKeysFileTable `json:"table"`
	Columns []indexesFileColumn  `json:"columns"`
	Unique  bool                 `json:"unique,omitempty"`
	Partial bool                 `json:"partial,omitempty"`
}

// indexesFileColumn is one column of an index.
type indexesFileColumn struct {
	Name       string `json:"name"`
	Descending bool   `json:"descending,omitempty"`
	Included   bool   `json:"included,omitempty"`
}

// LoadModelIndexes reads the indexes of the DB model modelID from the project
// folder projectDir: the tables that have indexes, in the order of the file
// (by schema, then table name), each with its indexes sorted by name. A model
// without the file has no stored indexes, which is no error. The tables carry
// their schema and name, and no catalog; an index carries its name, its
// columns in the order of the index, and its unique and partial flags, and no
// type.
func LoadModelIndexes(projectDir, modelID string) ([]datatug.TableIndexes, error) {
	return newFsDbModelsStore(projectDir).loadIndexes(modelID)
}

// SaveModelIndexes writes the indexes of the DB model modelID into the
// project folder projectDir, replacing the file whole. The same indexes give
// the same bytes whatever their order. An index that cannot be stored (see
// spec/features/model-indexes-file) is refused before the file is touched.
// Only the file of the indexes is written.
func SaveModelIndexes(projectDir, modelID string, tables []datatug.TableIndexes) error {
	return newFsDbModelsStore(projectDir).saveIndexes(modelID, tables)
}

// indexesFile returns the file of the indexes of model id, as a path inside
// the project (with slashes) and as a path on the machine, in the folder the
// foreign keys file is in (see foreignKeysFile).
func (s fsDbModelsStore) indexesFile(id string) (rel, full string) {
	return s.modelSideFile(id, storage.DbModelIndexesFileSuffix)
}

func validateIndexesModelID(id string) error {
	return validateModelSideFileID(id, "an indexes file")
}

func (s fsDbModelsStore) loadIndexes(id string) ([]datatug.TableIndexes, error) {
	if err := validateIndexesModelID(id); err != nil {
		return nil, err
	}
	rel, full := s.indexesFile(id)
	data, exists, err := readRegularFileCapped(full, maxIndexesFileSize)
	if err != nil {
		return nil, &modelFileError{msg: strings.ReplaceAll(err.Error(), full, rel), cause: err}
	}
	if !exists {
		return nil, nil
	}
	items, err := decodeIndexesFile(rel, data)
	if err != nil {
		return nil, err
	}
	return indexesFromItems(items), nil
}

func (s fsDbModelsStore) saveIndexes(id string, tables []datatug.TableIndexes) error {
	if err := validateIndexesModelID(id); err != nil {
		return err
	}
	rel, full := s.indexesFile(id)
	items := make([]indexesFileItem, 0)
	for _, table := range tables {
		for _, index := range table.Indexes {
			if index == nil {
				continue
			}
			items = append(items, indexItem(table.Table, index))
		}
	}
	items, err := checkAndSortIndexes(items)
	if err != nil {
		return &modelFileError{msg: fmt.Sprintf("%s: %v", rel, err), cause: err}
	}
	return plainfs.WriteFile(s.projectDir, full, func(w io.Writer) error {
		return encodeIndexesFile(w, items)
	})
}

// indexItem is the file's entry for index, an index of table. A column that
// is nil gets an empty name, which the checks refuse.
func indexItem(table datatug.DBCollectionKey, index *datatug.Index) indexesFileItem {
	columns := make([]indexesFileColumn, len(index.Columns))
	for i, c := range index.Columns {
		if c != nil {
			columns[i] = indexesFileColumn{Name: c.Name, Descending: c.IsDescending, Included: c.IsIncludedColumn}
		}
	}
	return indexesFileItem{
		Name:    index.Name,
		Table:   foreignKeysFileTable{Schema: table.Schema(), Name: table.Name()},
		Columns: columns,
		Unique:  index.IsUnique,
		Partial: index.IsPartial,
	}
}

// encodeIndexesFile writes items (checked and sorted) in the one layout the
// format has: tab indent, one member or element per line, no escaping of <, >
// and &, and a trailing newline.
func encodeIndexesFile(w io.Writer, items []indexesFileItem) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")
	return encoder.Encode(indexesFile{Version: indexesFileVersion, Indexes: items})
}

// decodeIndexesFile parses the file, whose path inside the project is rel,
// and returns its entries checked and sorted.
func decodeIndexesFile(rel string, data []byte) ([]indexesFileItem, error) {
	if err := checkModelSideFileHead(rel, data, indexesFileVersion); err != nil {
		return nil, err
	}
	var file indexesFileRead
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, notExpectedForm(rel, err)
	}
	if file.Indexes == nil {
		return nil, &modelFileError{msg: fmt.Sprintf(
			"%s has no \"indexes\" list (an empty list is \"indexes\": [])", rel)}
	}
	items, err := checkAndSortIndexes(*file.Indexes)
	if err != nil {
		return nil, &modelFileError{msg: fmt.Sprintf("%s: %v", rel, err), cause: err}
	}
	return items, nil
}

// checkAndSortIndexes refuses the first entry that breaks the checks of the
// format, and two entries of one name for one table, and returns the entries
// sorted by schema, table name and name, each compared as a string of bytes.
func checkAndSortIndexes(items []indexesFileItem) ([]indexesFileItem, error) {
	for i, item := range items {
		if err := item.check(i); err != nil {
			return nil, err
		}
	}
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, func(a, b indexesFileItem) int {
		return cmp.Or(
			cmp.Compare(a.Table.Schema, b.Table.Schema),
			cmp.Compare(a.Table.Name, b.Table.Name),
			cmp.Compare(a.Name, b.Name),
		)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Table == sorted[i].Table && sorted[i-1].Name == sorted[i].Name {
			return nil, fmt.Errorf("index %q of table %s is there twice", sorted[i].Name, sorted[i].Table)
		}
	}
	return sorted, nil
}

// check refuses an entry that the format does not allow; i is its position.
func (item indexesFileItem) check(i int) error {
	// Two tables can have an index of one name, so the table is named too.
	who := fmt.Sprintf("index at index %d of table %s", i, item.Table)
	if item.Name != "" {
		who = fmt.Sprintf("index %q of table %s at index %d", item.Name, item.Table, i)
	}
	switch {
	case !item.validText():
		return fmt.Errorf("%s has a name that is not valid UTF-8 text", who)
	case item.Name == "":
		return fmt.Errorf("%s has no name", who)
	case item.Table.Name == "":
		return fmt.Errorf("%s has no table name", who)
	case len(item.Columns) == 0:
		return fmt.Errorf("%s has no columns", who)
	case slices.ContainsFunc(item.Columns, func(c indexesFileColumn) bool { return c.Name == "" }):
		return fmt.Errorf("%s has an empty column name: an index on an expression cannot be stored", who)
	case !slices.ContainsFunc(item.Columns, func(c indexesFileColumn) bool { return !c.Included }):
		return fmt.Errorf("%s has only included columns and no key column", who)
	}
	return nil
}

// validText is false for an entry that has a name which is not valid UTF-8: a
// JSON encoder would silently write it as U+FFFD, which is another name.
func (item indexesFileItem) validText() bool {
	names := []string{item.Name, item.Table.Schema, item.Table.Name}
	for _, c := range item.Columns {
		names = append(names, c.Name)
	}
	return !slices.ContainsFunc(names, func(name string) bool { return !utf8.ValidString(name) })
}

// indexesFromItems groups the entries (sorted) by the table they are on. A
// table carries a schema and a name, no catalog.
func indexesFromItems(items []indexesFileItem) []datatug.TableIndexes {
	var tables []datatug.TableIndexes
	for i, item := range items {
		if i == 0 || items[i-1].Table != item.Table {
			tables = append(tables, datatug.TableIndexes{
				Table: datatug.NewTableKey(item.Table.Name, item.Table.Schema, "", nil),
			})
		}
		columns := make([]*datatug.IndexColumn, len(item.Columns))
		for j, c := range item.Columns {
			columns[j] = &datatug.IndexColumn{Name: c.Name, IsDescending: c.Descending, IsIncludedColumn: c.Included}
		}
		last := &tables[len(tables)-1]
		last.Indexes = append(last.Indexes, &datatug.Index{
			Name:      item.Name,
			Columns:   columns,
			IsUnique:  item.Unique,
			IsPartial: item.Partial,
		})
	}
	return tables
}

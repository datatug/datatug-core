package filestore

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// The foreign keys of a DB model are stored in one file beside the model's
// own file, "<model>.refs.json". The format is a public contract, specified in
// spec/features/model-foreign-keys-file; this file follows that page.

const (
	// foreignKeysFileVersion is the only version of the file this release
	// reads and writes.
	foreignKeysFileVersion = 1
	// maxForeignKeysFileSize is the biggest file a reader reads.
	maxForeignKeysFileSize int64 = 64 << 20
)

// foreignKeysFile is the content of "<model>.refs.json". The members are
// written in the order they are declared in.
type foreignKeysFile struct {
	Version     int                   `json:"version"`
	ForeignKeys []foreignKeysFileItem `json:"foreignKeys"`
}

// foreignKeysFileItem is one foreign key and the table that holds it.
type foreignKeysFileItem struct {
	Name       string               `json:"name"`
	Table      foreignKeysFileTable `json:"table"`
	Columns    []string             `json:"columns"`
	RefTable   foreignKeysFileTable `json:"refTable"`
	RefColumns []string             `json:"refColumns"`
}

// foreignKeysFileTable names a table. A table without a schema (SQLite) has
// no schema member.
type foreignKeysFileTable struct {
	Schema string `json:"schema,omitempty"`
	Name   string `json:"name"`
}

func (t foreignKeysFileTable) String() string {
	if t.Schema == "" {
		return t.Name
	}
	return t.Schema + "." + t.Name
}

// foreignKeysFileError is a refusal of the file. Its text names the file
// inside the project and never where the project is on the machine; cause,
// when there is one, is kept for errors.Is and errors.As.
type foreignKeysFileError struct {
	msg   string
	cause error
}

func (e *foreignKeysFileError) Error() string { return e.msg }
func (e *foreignKeysFileError) Unwrap() error { return e.cause }

// LoadModelForeignKeys reads the foreign keys of the DB model modelID from the
// project folder projectDir: the tables that hold keys, in the order of the
// file (by schema, then table name), each with its keys sorted by name. A model
// without the file has no stored foreign keys, which is no error. The tables
// and the referenced tables carry their schema and name, and no catalog.
func LoadModelForeignKeys(projectDir, modelID string) ([]datatug.TableForeignKeys, error) {
	return newFsDbModelsStore(projectDir).loadForeignKeys(modelID)
}

// SaveModelForeignKeys writes the foreign keys of the DB model modelID into
// the project folder projectDir, replacing the file whole. The same keys give
// the same bytes whatever their order. A key that cannot be stored (see
// spec/features/model-foreign-keys-file) is refused before the file is
// touched. Only the file of the foreign keys is written.
func SaveModelForeignKeys(projectDir, modelID string, tables []datatug.TableForeignKeys) error {
	return newFsDbModelsStore(projectDir).saveForeignKeys(modelID, tables)
}

// foreignKeysFile returns the file of the foreign keys of model id, as a path
// inside the project (with slashes) and as a path on the machine. It is in the
// folder the model's own file is found in: nested if "<id>/<id>.dbmodel.json"
// exists, else flat if "<id>.dbmodel.json" exists, else the nested folder a
// new model gets (see saveTarget).
func (s fsDbModelsStore) foreignKeysFile(id string) (rel, full string) {
	folder := id
	if _, err := os.Stat(s.nestedDbModelFilePath(id)); err != nil {
		if _, err = os.Stat(s.flatDbModelFilePath(id)); err == nil {
			folder = ""
		}
	}
	rel = path.Join(storage.DbModelsFolder, folder, storage.JsonFileName(id, storage.DbModelRefsFileSuffix))
	return rel, path.Join(s.projectDir, rel)
}

func validateForeignKeysModelID(id string) error {
	if id == "" || id == "." || id == ".." || strings.ContainsAny(id, `/\`) {
		return &foreignKeysFileError{msg: fmt.Sprintf(
			"db model id %q cannot name a foreign keys file: it is empty, is . or .., or has a path separator", id)}
	}
	return nil
}

func (s fsDbModelsStore) loadForeignKeys(id string) ([]datatug.TableForeignKeys, error) {
	if err := validateForeignKeysModelID(id); err != nil {
		return nil, err
	}
	rel, full := s.foreignKeysFile(id)
	data, exists, err := readRegularFileCapped(full, maxForeignKeysFileSize)
	if err != nil {
		return nil, &foreignKeysFileError{msg: strings.ReplaceAll(err.Error(), full, rel), cause: err}
	}
	if !exists {
		return nil, nil
	}
	items, err := decodeForeignKeysFile(rel, data)
	if err != nil {
		return nil, err
	}
	return foreignKeysFromItems(items), nil
}

func (s fsDbModelsStore) saveForeignKeys(id string, tables []datatug.TableForeignKeys) error {
	if err := validateForeignKeysModelID(id); err != nil {
		return err
	}
	rel, full := s.foreignKeysFile(id)
	items := make([]foreignKeysFileItem, 0)
	for _, table := range tables {
		for _, fk := range table.ForeignKeys {
			if fk == nil {
				continue
			}
			item, err := foreignKeyItem(table.Table, fk)
			if err != nil {
				return &foreignKeysFileError{msg: fmt.Sprintf("%s: %v", rel, err), cause: err}
			}
			items = append(items, item)
		}
	}
	items, err := checkAndSortForeignKeys(items)
	if err != nil {
		return &foreignKeysFileError{msg: fmt.Sprintf("%s: %v", rel, err), cause: err}
	}
	return plainfs.WriteFile(s.projectDir, full, func(w io.Writer) error {
		return encodeForeignKeysFile(w, items)
	})
}

// foreignKeyItem is the file's entry for fk, a key of table. It refuses a key
// the file cannot say: one that points to a table of another catalog.
func foreignKeyItem(table datatug.DBCollectionKey, fk *datatug.ForeignKey) (foreignKeysFileItem, error) {
	if c, rc := table.Catalog(), fk.RefTable.Catalog(); c != "" && rc != "" && c != rc {
		return foreignKeysFileItem{}, fmt.Errorf(
			"foreign key %q of table %q points to a table of another catalog (%q, not %q): the file cannot say it",
			fk.Name, table.Name(), rc, c)
	}
	return foreignKeysFileItem{
		Name:       fk.Name,
		Table:      foreignKeysFileTable{Schema: table.Schema(), Name: table.Name()},
		Columns:    fk.Columns,
		RefTable:   foreignKeysFileTable{Schema: fk.RefTable.Schema(), Name: fk.RefTable.Name()},
		RefColumns: fk.RefColumns,
	}, nil
}

// encodeForeignKeysFile writes items (checked and sorted) in the one layout
// the format has: tab indent, one member or element per line, no escaping of
// <, > and &, and a trailing newline.
func encodeForeignKeysFile(w io.Writer, items []foreignKeysFileItem) error {
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "\t")
	return encoder.Encode(foreignKeysFile{Version: foreignKeysFileVersion, ForeignKeys: items})
}

// decodeForeignKeysFile parses the file, whose path inside the project is
// rel, and returns its entries checked and sorted.
func decodeForeignKeysFile(rel string, data []byte) ([]foreignKeysFileItem, error) {
	// The version is read alone first, so that a file of a later version, which
	// may have another shape, is refused for its version.
	var head struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &head); err != nil {
		return nil, notExpectedForm(rel, err)
	}
	switch {
	case head.Version == nil:
		return nil, &foreignKeysFileError{msg: fmt.Sprintf(
			"%s has no \"version\"; this release reads version %d", rel, foreignKeysFileVersion)}
	case *head.Version != foreignKeysFileVersion:
		return nil, &foreignKeysFileError{msg: fmt.Sprintf(
			"%s is version %d, which this release does not know (it reads version %d); update DataTug to read it",
			rel, *head.Version, foreignKeysFileVersion)}
	}
	var file foreignKeysFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, notExpectedForm(rel, err)
	}
	items, err := checkAndSortForeignKeys(file.ForeignKeys)
	if err != nil {
		return nil, &foreignKeysFileError{msg: fmt.Sprintf("%s: %v", rel, err), cause: err}
	}
	return items, nil
}

func notExpectedForm(rel string, err error) error {
	return &foreignKeysFileError{
		msg:   fmt.Sprintf("%s is not a JSON object of the expected form: %v", rel, err),
		cause: err,
	}
}

// checkAndSortForeignKeys refuses the first entry that breaks the checks of the
// format, and two entries of one name for one table, and returns the entries
// sorted by schema, table name and name, each compared as a string of bytes.
func checkAndSortForeignKeys(items []foreignKeysFileItem) ([]foreignKeysFileItem, error) {
	for i, item := range items {
		if err := item.check(i); err != nil {
			return nil, err
		}
	}
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, func(a, b foreignKeysFileItem) int {
		return cmp.Or(
			cmp.Compare(a.Table.Schema, b.Table.Schema),
			cmp.Compare(a.Table.Name, b.Table.Name),
			cmp.Compare(a.Name, b.Name),
		)
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Table == sorted[i].Table && sorted[i-1].Name == sorted[i].Name {
			return nil, fmt.Errorf("foreign key %q of table %s is there twice", sorted[i].Name, sorted[i].Table)
		}
	}
	return sorted, nil
}

// check refuses an entry that the format does not allow; i is its position.
func (item foreignKeysFileItem) check(i int) error {
	who := fmt.Sprintf("foreign key at index %d", i)
	if item.Name != "" {
		who = fmt.Sprintf("foreign key %q at index %d", item.Name, i)
	}
	switch {
	case item.Name == "":
		return fmt.Errorf("%s has no name", who)
	case item.Table.Name == "":
		return fmt.Errorf("%s has no table name", who)
	case len(item.Columns) == 0:
		return fmt.Errorf("%s has no columns", who)
	case slices.Contains(item.Columns, ""):
		return fmt.Errorf("%s has an empty column name", who)
	case item.RefTable.Name == "":
		return fmt.Errorf("%s has no referenced table name", who)
	case len(item.RefColumns) == 0:
		return fmt.Errorf("%s has no referenced columns: a source that does not report the referenced columns gives a key that cannot be stored", who)
	case len(item.RefColumns) != len(item.Columns):
		return fmt.Errorf("%s has %d columns but %d referenced columns", who, len(item.Columns), len(item.RefColumns))
	case slices.Contains(item.RefColumns, ""):
		return fmt.Errorf("%s has an empty referenced column name", who)
	}
	return nil
}

// foreignKeysFromItems groups the entries (sorted) by the table that holds
// them. A table and a referenced table carry a schema and a name, no catalog.
func foreignKeysFromItems(items []foreignKeysFileItem) []datatug.TableForeignKeys {
	var tables []datatug.TableForeignKeys
	for i, item := range items {
		if i == 0 || items[i-1].Table != item.Table {
			tables = append(tables, datatug.TableForeignKeys{
				Table: datatug.NewTableKey(item.Table.Name, item.Table.Schema, "", nil),
			})
		}
		last := &tables[len(tables)-1]
		last.ForeignKeys = append(last.ForeignKeys, &datatug.ForeignKey{
			Name:       item.Name,
			Columns:    item.Columns,
			RefTable:   datatug.NewTableKey(item.RefTable.Name, item.RefTable.Schema, "", nil),
			RefColumns: item.RefColumns,
		})
	}
	return tables
}

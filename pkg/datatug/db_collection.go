package datatug

import "fmt"

// CollectionInfo holds metadata about a collection or a table or a view
type CollectionInfo struct {
	DBCollectionKey
	RecordsetBaseDef
	TableProps
	DLL          string        `json:"dll,omitempty"` // Data Definition Language
	Columns      TableColumns  `json:"columns,omitempty"`
	Indexes      []*Index      `json:"indexes,omitempty"`
	ReferencedBy ReferencedBys `json:"referencedBy,omitempty"`
	RecordsCount *int          `json:"recordsCount,omitempty"`
}

// Validate returns error if not valid
func (v CollectionInfo) Validate() error {
	if err := v.DBCollectionKey.Validate(); err != nil {
		return err
	}
	if err := v.TableProps.Validate(); err != nil {
		return err
	}
	if err := v.PrimaryKey.Validate(); err != nil {
		return fmt.Errorf("invalid primary key: %w", err)
	}
	if err := v.Columns.Validate(); err != nil {
		return err
	}
	if err := v.ForeignKeys.Validate(); err != nil {
		return err
	}
	return nil
}

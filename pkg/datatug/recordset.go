package datatug

import (
	"fmt"
	"time"

	"github.com/strongo/validation"
)

// Recordset holds data & stats for recordset returned by executed command
type Recordset struct {
	Duration time.Duration     `json:"durationNanoseconds"`
	Columns  []RecordsetColumn `json:"columns"`
	Rows     [][]interface{}   `json:"rows"`
}

// Validate returns error if not valid
func (v Recordset) Validate() error {
	for _, c := range v.Columns {
		if err := c.Validate(); err != nil {
			return err
		}
	}
	for i, row := range v.Rows {
		if row == nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("rows[%v]", i), "is nil")
		}
	}
	return nil
}

// RecordsetColumn describes column in a recordset
type RecordsetColumn struct {
	Name   string          `json:"name"`
	DbType string          `json:"dbType"`
	Meta   *EntityFieldRef `json:"meta"`
}

// Validate returns error if not valid
func (v RecordsetColumn) Validate() error {
	if err := validateStringField("name", v.Name, true, 100); err != nil {
		return err
	}
	if v.Meta != nil {
		if err := v.Meta.Validate(); err != nil {
			return err
		}
	}
	return nil
}

package models

import "time"

// Recordset holds data & stats for recordset returned by executed command
type Recordset struct {
	Duration time.Duration     `json:"durationNanoseconds"`
	Columns  []RecordsetColumn `json:"columns"`
	Rows     [][]interface{}   `json:"rows"`
}

// RecordsetColumn describes column in a recordset
type RecordsetColumn struct {
	Name   string          `json:"name"`
	DbType string          `json:"dbType"`
	Meta   *EntityFieldRef `json:"meta"`
}

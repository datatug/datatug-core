package dto

import "github.com/datatug/datatug-core/pkg/datatug"

// ProjRecordsetSummary holds summary info about recordset definition
type ProjRecordsetSummary struct {
	datatug.ProjectItem
	Columns    []string                `json:"columns,omitempty"`
	Recordsets []*ProjRecordsetSummary `json:"recordsets,omitempty"`
}

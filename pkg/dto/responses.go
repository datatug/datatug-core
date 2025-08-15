package dto

import "github.com/datatug/datatug-core/pkg/models"

// ProjRecordsetSummary holds summary info about recordset definition
type ProjRecordsetSummary struct {
	models.ProjectItem
	Columns    []string                `json:"columns,omitempty"`
	Recordsets []*ProjRecordsetSummary `json:"recordsets,omitempty"`
}

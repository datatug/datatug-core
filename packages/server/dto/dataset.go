package dto

type ProjRecordsetSummary struct {
	ID         string                 `json:"id"`
	//
	Title      string                 `json:"title,omitempty"`
	Columns    []string               `json:"columns,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	//
	Recordsets []*ProjRecordsetSummary `json:"recordsets,omitempty"`
}

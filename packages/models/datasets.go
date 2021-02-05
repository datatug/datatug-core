package models

// DatasetDef is a set of recordsets
type DatasetDef struct {
	Recordsets []DatasetRefToRecordset `json:"recordsets"`
}

// DatasetRefToRecordset is a reference from dataset to recordset definition and settings specific for the dataset
type DatasetRefToRecordset struct {
	MinRecordsCount int `json:"minRecordsCount:omitempty"`
	MaxRecordsCount int `json:"maxRecordsCount:omitempty"`
}

package models

// DatabaseDifferences holds DB diffs
type DatabaseDifferences struct {
	ID          string      `json:"id"`
	SchemasDiff SchemasDiff `json:"schemasDiff"`
}

// PropertyDiff holds diffs about some property
type PropertyDiff struct {
	Name string `json:"name"`
	HitAndMiss
}

// DiffDbRef is a link to DB
type DiffDbRef struct {
	Environment string `json:"env"`
	Host        string `json:"host"`
	Port        int    `json:"port,omitempty"`
	Catalog     string `json:"catalog,omitempty"`
}

// HitAndMiss stores where entity exists and where not
type HitAndMiss struct {
	IsInModel bool        `json:"isInModel"`
	ExistsIn  []DiffDbRef `json:"existsIn"` // points to DatabaseDifferences.DatabaseDiffs[]DatabaseDiff.ID
	MissingIn []DiffDbRef `json:"missingIn"`
}

// SchemasDiff holds list of schema diffs
type SchemasDiff []SchemaDiff

// SchemaDiff holds schema diffs
type SchemaDiff struct {
	HitAndMiss
	TablesDiff TablesDiff `json:"tablesDiff"`
	ViewsDiff  TablesDiff `json:"viewsDiff"`
}

// TablesDiff holds list of table diffs
type TablesDiff []TableDiff

// TableDiff holds table diffs
type TableDiff struct {
	HitAndMiss
	ColumnsDiff ColumnsDiff `json:"columnsDiff"`
}

// ColumnsDiff holds list of column diffs
type ColumnsDiff []ColumnDiff

// ColumnDiff holds column diffs
type ColumnDiff struct {
	HitAndMiss
}

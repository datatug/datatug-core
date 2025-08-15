package schemer

import "database/sql"

// TablePropsReader reads table props
type TablePropsReader struct {
	Table string
	Rows  *sql.Rows
}

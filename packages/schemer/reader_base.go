package schemer

import "database/sql"

type TablePropsReader struct {
	Table string
	Rows  *sql.Rows
}

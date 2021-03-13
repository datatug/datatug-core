package dbconnection

const DriverSQLite3 = "sqlite3"

var _ Params = (*SQLite3ConnectionParams)(nil)

func NewSQLite3ConnectionParams(path, catalog string, mode Mode) SQLite3ConnectionParams {
	return SQLite3ConnectionParams{path: path, catalog: catalog, mode: mode}
}

// SQLite3ConnectionParams
type SQLite3ConnectionParams struct {
	catalog string
	path    string
	mode    Mode
}

func (SQLite3ConnectionParams) Driver() string {
	return DriverSQLite3
}

func (SQLite3ConnectionParams) Server() string {
	return "localhost"
}

func (SQLite3ConnectionParams) Port() int {
	return 0
}

func (v SQLite3ConnectionParams) Mode() string {
	return v.mode
}

func (v SQLite3ConnectionParams) Path() string {
	return v.path
}

func (v SQLite3ConnectionParams) Catalog() string {
	return v.catalog
}

func (v SQLite3ConnectionParams) User() string {
	return ""
}

func (v SQLite3ConnectionParams) String() string {
	s := "file:" + v.path
	return s
}

func (v SQLite3ConnectionParams) ConnectionString() string {
	s := v.String()
	return s
}

package dbconnection

// DriverSQLite3 defines SQLite driver name
const DriverSQLite3 = "sqlite3"

var _ Params = (*SQLite3ConnectionParams)(nil)

// NewSQLite3ConnectionParams creates new SQLite connection params
func NewSQLite3ConnectionParams(path, catalog string, mode Mode) SQLite3ConnectionParams {
	return SQLite3ConnectionParams{path: path, catalog: catalog, mode: mode}
}

// SQLite3ConnectionParams defines SQLite connection params
type SQLite3ConnectionParams struct {
	catalog string
	path    string
	mode    Mode
}

// Driver returns driver
func (SQLite3ConnectionParams) Driver() string {
	return DriverSQLite3
}

// Server returns server
func (SQLite3ConnectionParams) Server() string {
	return "localhost"
}

// Port returns port
func (SQLite3ConnectionParams) Port() int {
	return 0
}

// Mode returns mode
func (v SQLite3ConnectionParams) Mode() string {
	return v.mode
}

// Path returns path
func (v SQLite3ConnectionParams) Path() string {
	return v.path
}

// Catalog returns catalog
func (v SQLite3ConnectionParams) Catalog() string {
	return v.catalog
}

// User returns user
func (v SQLite3ConnectionParams) User() string {
	return ""
}

// String serializes to string
func (v SQLite3ConnectionParams) String() string {
	s := "file:" + v.path
	return s
}

// ConnectionString returns connection string
func (v SQLite3ConnectionParams) ConnectionString() string {
	s := v.String()
	return s
}

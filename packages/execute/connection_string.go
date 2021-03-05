package execute

import (
	"fmt"
	"github.com/datatug/datatug/packages/dbconnection"
	"github.com/strongo/validation"
	"strconv"
	"strings"
)

var _ dbconnection.Params = (*ConnectionString)(nil)

// ConnectionString hold connection parameters
type ConnectionString struct {
	mode     dbconnection.Mode
	driver   string
	server   string
	port     int
	catalog  string
	user     string
	password string
	path     string
}

func (v ConnectionString) Mode() dbconnection.Mode {
	return v.mode
}

func (v ConnectionString) Catalog() string {
	panic("implement me")
}

func (v ConnectionString) ConnectionString() string {
	panic("implement me")
}

// Driver returns DB
func (v ConnectionString) Driver() string {
	return v.driver
}

// Database returns DB
func (v ConnectionString) Database() string {
	return v.catalog
}

// Server returns server
func (v ConnectionString) Server() string {
	return v.server
}

// Path returns path to file (for SQLite3)
func (v ConnectionString) Path() string {
	return v.path
}

// Port returns port
func (v ConnectionString) Port() int {
	return v.port
}

// User returns user
func (v ConnectionString) User() string {
	return v.user
}

// NewConnectionString creates new connection parameters
func NewConnectionString(driver, server, user, password, database string, options ...string) (connectionString ConnectionString, err error) {
	connectionString = ConnectionString{
		driver:   driver,
		server:   server,
		catalog:  database,
		user:     user,
		password: password,
	}

	for _, o := range options {
		eqIndex := strings.Index(o, "=")
		name := o[:eqIndex]
		v := o[eqIndex+1:]
		switch name {
		case "path":
			connectionString.path = v
		case "port":
			if connectionString.port, err = strconv.Atoi(v); err != nil {
				return
			}
		case "mode":
			switch v {
			case dbconnection.ModeReadOnly, dbconnection.ModeReadWrite:
				connectionString.mode = v // OK
			default:
				err = validation.NewErrBadRequestFieldValue("mode", fmt.Sprintf("unsupported value, expected [%v, %v] but got: %v", dbconnection.ModeReadOnly, dbconnection.ModeReadWrite, v))
				return
			}
		}
	}

	return
}

// String serializes connection parameters to a string
func (v ConnectionString) String() string {
	switch v.driver {
	case "sqlite3":
		return v.forSQLite3()
	default:
		return v.def()
	}
}

func (v ConnectionString) forSQLite3() string {
	return fmt.Sprintf("file:%v", v.path)
}

func (v ConnectionString) def() string {
	connectionParams := make([]string, 0, 8)
	connectionParams = append(connectionParams, "server="+v.server)
	//connectionParams = append(connectionParams, fmt.Sprintf("ServerSPN=MSSQLSvc/%v:1433", v.server))
	if v.port != 0 {
		connectionParams = append(connectionParams, "port="+strconv.Itoa(v.port))
	}
	if v.user != "" {
		connectionParams = append(connectionParams, "user id="+v.user)
		if v.password != "" {
			connectionParams = append(connectionParams, "password="+v.password)
		}
	} else {
		connectionParams = append(connectionParams, "trusted_connection=yes")
	}

	if v.catalog != "" {
		connectionParams = append(connectionParams, "database="+v.catalog)
	}
	//return fmt.Sprintf(
	//	"sqlserver://%v?Connection+Timeout=30&Database=%v&Integrated+Security=SSPI&TrustServerCertificate=true&encrypt=true",
	//	v.server,
	//	v.database,
	//	)

	return strings.Join(connectionParams, ";")
}

package dbconnection

import (
	"fmt"
	"github.com/strongo/validation"
	"strconv"
	"strings"
)

var _ Params = (*GeneralParams)(nil)

// GeneralParams hold connection parameters
type GeneralParams struct {
	mode     Mode
	driver   string
	server   string
	port     int
	catalog  string
	user     string
	password string
	path     string
}

// Mode returns mode
func (v GeneralParams) Mode() Mode {
	return v.mode
}

// Catalog returns catalog
func (v GeneralParams) Catalog() string {
	panic("implement me")
}

// ConnectionString returns ConnectionString
func (v GeneralParams) ConnectionString() string {
	panic("implement me")
}

// Driver returns DB
func (v GeneralParams) Driver() string {
	return v.driver
}

// Database returns DB
func (v GeneralParams) Database() string {
	return v.catalog
}

// Server returns server
func (v GeneralParams) Server() string {
	return v.server
}

// Path returns path to file (for SQLite3)
func (v GeneralParams) Path() string {
	return v.path
}

// Port returns port
func (v GeneralParams) Port() int {
	return v.port
}

// User returns user
func (v GeneralParams) User() string {
	return v.user
}

// NewConnectionString creates new connection parameters
func NewConnectionString(driver, server, user, password, database string, options ...string) (connectionString GeneralParams, err error) {
	connectionString = GeneralParams{
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
			case ModeReadOnly, ModeReadWrite:
				connectionString.mode = v // OK
			default:
				err = validation.NewErrBadRequestFieldValue("mode", fmt.Sprintf("unsupported value, expected [%v, %v] but got: %v", ModeReadOnly, ModeReadWrite, v))
				return
			}
		}
	}

	return
}

// String serializes connection parameters to a string
func (v GeneralParams) String() string {
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


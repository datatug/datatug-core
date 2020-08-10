package execute

import (
	"strconv"
	"strings"
)

// ConnectionString hold connection parameters
type ConnectionString struct {
	database string
	server   string
	port     int
	user     string
	password string
}

// Database return DB
func (v ConnectionString) Database() string {
	return v.database
}

// Server return server
func (v ConnectionString) Server() string {
	return v.server
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
func NewConnectionString(server, user, password, database string, port int) ConnectionString {
	return ConnectionString{
		server:   server,
		port:     port,
		user:     user,
		password: password,
		database: database,
	}
}

// String serializes connection parameters to a string
func (v ConnectionString) String() string {
	connectionParams := make([]string, 0, 8)
	connectionParams = append(connectionParams, "server="+v.server)
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

	if v.database != "" {
		connectionParams = append(connectionParams, "database="+v.database)
	}

	return strings.Join(connectionParams, ";")
}

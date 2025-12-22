package dbconnection

import (
	"testing"
)

func TestNewSQLite3ConnectionParams(t *testing.T) {
	path := "/path/to/db.sqlite"
	catalog := "main"
	mode := ModeReadOnly
	params := NewSQLite3ConnectionParams(path, catalog, mode)

	if params.Driver() != DriverSQLite3 {
		t.Errorf("expected driver %v, got %v", DriverSQLite3, params.Driver())
	}
	if params.Path() != path {
		t.Errorf("expected path %v, got %v", path, params.Path())
	}
	if params.Catalog() != catalog {
		t.Errorf("expected catalog %v, got %v", catalog, params.Catalog())
	}
	if params.Mode() != mode {
		t.Errorf("expected mode %v, got %v", mode, params.Mode())
	}
	if params.Server() != "localhost" {
		t.Errorf("expected server localhost, got %v", params.Server())
	}
	if params.Port() != 0 {
		t.Errorf("expected port 0, got %v", params.Port())
	}
	if params.User() != "" {
		t.Errorf("expected empty user, got %v", params.User())
	}

	expectedString := "file:" + path
	if params.String() != expectedString {
		t.Errorf("expected string %v, got %v", expectedString, params.String())
	}
	if params.ConnectionString() != expectedString {
		t.Errorf("expected connection string %v, got %v", expectedString, params.ConnectionString())
	}
}

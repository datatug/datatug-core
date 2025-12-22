package dbconnection

import "testing"

func TestGeneralParams_Catalog(t *testing.T) {
	expected := "TestCatalog"
	v := GeneralParams{catalog: expected}
	if v.Catalog() != expected {
		t.Error("Unexpected catalog value")
	}
}

func TestNewConnectionString(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		driver := "sqlserver"
		server := "localhost"
		user := "sa"
		password := "password"
		database := "master"
		params, err := NewConnectionString(driver, server, user, password, database)
		if err != nil {
			t.Fatalf("failed to create connection string params: %v", err)
		}
		if params.Driver() != driver {
			t.Errorf("expected driver %v, got %v", driver, params.Driver())
		}
		if params.Server() != server {
			t.Errorf("expected server %v, got %v", server, params.Server())
		}
		if params.User() != user {
			t.Errorf("expected user %v, got %v", user, params.User())
		}
		if params.Database() != database {
			t.Errorf("expected database %v, got %v", database, params.Database())
		}
	})

	t.Run("with_options", func(t *testing.T) {
		params, err := NewConnectionString("sqlserver", "localhost", "sa", "password", "master", "port=1433", "path=/tmp/db", "mode=ro")
		if err != nil {
			t.Fatalf("failed to create connection string params: %v", err)
		}
		if params.Port() != 1433 {
			t.Errorf("expected port 1433, got %v", params.Port())
		}
		if params.Path() != "/tmp/db" {
			t.Errorf("expected path /tmp/db, got %v", params.Path())
		}
		if params.Mode() != ModeReadOnly {
			t.Errorf("expected mode %v, got %v", ModeReadOnly, params.Mode())
		}
	})

	t.Run("invalid_port", func(t *testing.T) {
		_, err := NewConnectionString("sqlserver", "localhost", "sa", "password", "master", "port=abc")
		if err == nil {
			t.Error("expected error for invalid port")
		}
	})

	t.Run("invalid_mode", func(t *testing.T) {
		_, err := NewConnectionString("sqlserver", "localhost", "sa", "password", "master", "mode=invalid")
		if err == nil {
			t.Error("expected error for invalid mode")
		}
	})
}

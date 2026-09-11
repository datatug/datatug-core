package datatug

import (
	"strings"
	"testing"
)

// Review S4: credential screening must refuse a password in every common
// connection-string syntax while still allowing a username alone.

func TestEmbeddedCredentialReason_Refused(t *testing.T) {
	for _, value := range []string{
		// URL userinfo with a password.
		"postgres://u:secret@h/db",
		"postgres://u:secret@h",
		"https://user:p%40ss@h",   // percent-encoded password
		"postgres://:secret@h/db", // password with an empty username
		"postgres://u:@h/db",      // an explicitly present, empty password
		"jdbc:postgresql://u:secret@h:5432/db",
		"see postgres://u:secret@h/db for details",
		"Url=postgres://u:secret@h;Other=1", // net/url refuses the host; the fallback still catches it
		// DSN userinfo without a "//" authority.
		"user:pass@tcp(localhost:3306)/db",
		"user:pass@tcp(localhost:3306)/db?parseTime=true",
		"admin:hunter2@db.example.com",
		"Data Source=user:pass@h",
		// Key/value connection strings and URL query parameters.
		"Server=h;User Id=u;Password=secret;",
		"Server=h;PWD=secret",
		"password=secret",
		"Password = 'secret'",
		"host=h user=u passwd=secret",
		"Server=h;ssl_password=secret",
		"postgres://u@h/db?password=secret",
		"postgres://u@h/db?sslmode=require&password=secret",
	} {
		if _, found := embeddedCredentialReason(value); !found {
			t.Errorf("expected %q to be refused as an embedded credential", value)
		}
	}
}

func TestEmbeddedCredentialReason_Allowed(t *testing.T) {
	for _, value := range []string{
		"",
		"h",
		"db.example.com",
		"h:5432",
		"postgres://u@h/db", // a username alone may identify a connection
		"postgres://h/db",
		"jdbc:postgresql://u@h:5432/db",
		"https://h/path?x=1&y=2",
		"u@tcp(localhost:3306)/db",
		"tcp(localhost:3306)/db",
		"Server=h;User Id=u;",
		"Server=h;User Id=u@corp;",
		"Server=h;Password=;", // an empty password holds no secret
		"user@example.com",
		"Time: 10:00 @ office",
		"forward=1",
		"SELECT 1",
	} {
		if reason, found := embeddedCredentialReason(value); found {
			t.Errorf("expected %q to be allowed, got refused: %s", value, reason)
		}
	}
}

func TestQueryDefTarget_Validate_CredentialsInEveryConnectionField(t *testing.T) {
	const secret = "Server=h;User Id=u;Password=secret;"
	for name, target := range map[string]QueryDefTarget{
		"driver":   {Driver: secret},
		"catalog":  {Catalog: secret},
		"protocol": {Protocol: "postgres://u:secret@h"},
		"host":     {Host: secret},
	} {
		err := target.Validate()
		if err == nil {
			t.Errorf("%s: expected a credential to be refused", name)
			continue
		}
		if !strings.Contains(err.Error(), name) {
			t.Errorf("%s: expected the error to name the field, got: %v", name, err)
		}
	}
	for name, target := range map[string]QueryDefTarget{
		"username field":    {Host: "h", Credentials: Credentials{Username: "reader"}},
		"username in a URL": {Host: "postgres://reader@h/db"},
		"host and port":     {Host: "h", Port: 5432, Driver: "postgres"},
	} {
		if err := target.Validate(); err != nil {
			t.Errorf("%s: expected no error, got: %v", name, err)
		}
	}
}

func TestQueryDef_Validate_ScreensStringParameterDefaults(t *testing.T) {
	refused := []any{
		"postgres://u:secret@h/db",
		"Server=h;Password=secret",
		[]string{"ok", "user:pass@tcp(h)/db"},
		[]any{"ok", "postgres://u:secret@h/db"},
	}
	for _, dv := range refused {
		v := newQueryDef("SQL", "SELECT 1")
		v.Parameters = Parameters{{ID: "dsn", Type: "string", DefaultValue: dv}}
		if _, isString := dv.(string); !isString {
			// A multi-value default is not a plain "string" typed default.
			v.Parameters[0].Type = "text"
		}
		err := v.Validate()
		if err == nil {
			t.Errorf("expected parameter default %#v to be refused", dv)
			continue
		}
		if !strings.Contains(err.Error(), "parameters[0].defaultValue") {
			t.Errorf("expected the error to name the parameter default, got: %v", err)
		}
	}

	allowed := []any{"postgres://u@h/db", "hello", "user@example.com", 42, nil}
	for _, dv := range allowed {
		v := newQueryDef("SQL", "SELECT 1")
		v.Parameters = Parameters{{ID: "p", Type: "any", DefaultValue: dv}}
		if err := v.Validate(); err != nil {
			t.Errorf("expected parameter default %#v to be allowed, got: %v", dv, err)
		}
	}
}

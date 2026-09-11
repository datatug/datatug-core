package datatug

import (
	"strings"
	"testing"
)

// Review S4 and SF1: credential screening must refuse a secret in every
// connection-string syntax it documents while still allowing a username
// alone and ordinary values that only mention a secret's name.

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
		"mongodb://u:secret@h1,h2,h3/db?replicaSet=rs",
		"redis://:secret@h:6379",
		// SF1: an unencoded "/", "#" or "?" in the password makes net/url
		// fail; the userinfo fallback still refuses it.
		"postgres://user:pa/ss@host/db",
		"postgres://user:pa#ss@host/db",
		"postgres://user:pa?ss@host/db",
		"mysql://user:p@ss@host/db", // an unencoded "@" in the password
		// DSN userinfo without a "//" authority.
		"user:pass@tcp(localhost:3306)/db",
		"user:pass@tcp(localhost:3306)/db?parseTime=true",
		"admin:hunter2@db.example.com",
		"Data Source=user:pass@h",
		"u:secret@tcp(h:3306)/db",
		// Key/value connection strings and URL query parameters.
		"Server=h;User Id=u;Password=secret;",
		"Server=h;PWD=secret",
		"password=secret",
		"Password = 'secret'",
		"host=h user=u passwd=secret",
		"host=h user=u password=secret",
		"Server=h;ssl_password=secret",
		"postgres://u@h/db?password=secret",
		"postgres://u@h/db?sslmode=require&password=secret",
		"jdbc:databricks://h:443/default;AuthMech=3;UID=token;PWD=dapi123",
		// SF1: Azure, token, API key, SAS and Pass= forms.
		"DefaultEndpointsProtocol=https;AccountName=a;AccountKey=c2VjcmV0;EndpointSuffix=core.windows.net",
		"Endpoint=sb://ns.servicebus.windows.net/;SharedAccessKeyName=Root;SharedAccessKey=abc",
		"Endpoint=sb://ns.servicebus.windows.net/;SharedAccessSignature=SharedAccessSignature sr=x&sig=y",
		"md:?motherduck_token=eyJhbGciOi",
		"https://h/api?api_key=abc123",
		"https://h/api?apikey=abc123",
		"https://h/api?access_token=abc123",
		"snowflake://u@acct/db?token=abc&authenticator=oauth",
		"https://acct.blob.core.windows.net/c?sv=2020&sig=abc%3D",
		"https://login.example.com/token?client_id=x&client_secret=y",
		"Server=h;Uid=u;Pass=secret",
		// SF1: JSON members.
		`{"user":"u","password":"secret"}`,
		`{"api_key": "abc123"}`,
		// HTTP credential headers.
		"GET https://api.example.com/x\nAuthorization: Bearer abc",
		"Authorization: Basic dXNlcjpwYXNz",
		"authorization: rawtoken123",
		"X-API-Key: abc123",
	} {
		if _, found := EmbeddedCredentialReason(value); !found {
			t.Errorf("expected %q to be refused as an embedded credential", value)
		}
	}
}

func TestEmbeddedCredentialReason_Allowed(t *testing.T) {
	for _, value := range []string{
		"",
		"h",
		"db.example.com",
		"db.example.com:5432",
		"[::1]:5432",
		"h:5432",
		"github.com/jackc/pgx/v5",
		"git@github.com:org/repo.git",
		"ssh://git@github.com:22/x",
		"http://example.com/a:b@c",
		"postgres://u@h/db", // a username alone may identify a connection
		"postgres://u@h/db?sslmode=require",
		"postgres://h/db",
		"jdbc:postgresql://u@h:5432/db",
		"https://h/path?x=1&y=2",
		"u@tcp(localhost:3306)/db",
		"tcp(localhost:3306)/db",
		"Server=h;User Id=u;",
		"Server=h;User Id=u@corp;",
		"Server=h;Uid=reader;",
		"Server=h;Password=;", // an empty password holds no secret
		"Endpoint=sb://ns.servicebus.windows.net/;SharedAccessKeyName=Root",
		"user@example.com",
		"Time: 10:00 @ office",
		"forward=1",
		"SELECT 1",
		"?passwordless=true",
		"password_reset=1",
		"2024-01-01T10:00:00Z",
		// SF1 false-positive guards and the review's nits.
		"mailto:a@b.com",
		"mailto:someone@example.com?subject=hi",
		"sip:alice@example.com",
		"10:30@home",
		"reset_password=true",
		"token_count=5",
		"max_tokens=100",
		"https://api.example.com/items?page_token=abc&pageSize=10",
		"https://api.example.com/items?nextToken=abc",
		`{"token_count": 5, "password_policy": "strict", "is_admin": true}`,
		`{"password": ""}`,
		`{"password": "{{password}}"}`,
		// Ordinary SQL that mentions a password column.
		"SELECT id, password_hash FROM users WHERE password_hash IS NOT NULL",
		"SELECT password FROM users",
		"UPDATE users SET password = :new_password WHERE id = :id",
		"SELECT * FROM users WHERE password = ?",
		"SELECT * FROM users WHERE password = $1",
		// Placeholders and environment references.
		"Server=h;Password=${DB_PASSWORD};",
		"Server=h;Password=$DB_PASSWORD",
		"password=<your-password>",
		"Authorization: Bearer {{token}}",
		"Authorization: Bearer $API_TOKEN",
		"Authorization: Bearer",
		"X-API-Key: ${API_KEY}",
	} {
		if reason, found := EmbeddedCredentialReason(value); found {
			t.Errorf("expected %q to be allowed, got refused: %s", value, reason)
		}
	}
}

// The documented limits stay what the package doc says they are.
func TestEmbeddedCredentialReason_DocumentedLimits(t *testing.T) {
	for _, value := range []string{"scott/tiger@orcl", "https://user:123/x@h", "?key=AIzaSyA", "password: secret"} {
		if _, found := EmbeddedCredentialReason(value); found {
			t.Errorf("%q is documented as not detected; update the package doc if that changed", value)
		}
	}
	for _, value := range []string{"SELECT * FROM users WHERE password = 'x'", "a,b:c@d"} {
		if _, found := EmbeddedCredentialReason(value); !found {
			t.Errorf("%q is documented as refused; update the package doc if that changed", value)
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
		"username": {Host: "h", Credentials: Credentials{Username: "postgres://u:secret@h"}},
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
		"username with @":   {Host: "h", Credentials: Credentials{Username: "reader@corp"}},
		"username in a URL": {Host: "postgres://reader@h/db"},
		"host and port":     {Host: "h", Port: 5432, Driver: "postgres"},
	} {
		if err := target.Validate(); err != nil {
			t.Errorf("%s: expected no error, got: %v", name, err)
		}
	}
}

func TestQueryDef_Validate_ScreensParameterDefaults(t *testing.T) {
	refused := []struct {
		typ string
		dv  any
	}{
		{"string", "postgres://u:secret@h/db"},
		{"string", "Server=h;Password=secret"},
		{"text", []string{"ok", "user:pass@tcp(h)/db"}},
		{"text", []any{"ok", "postgres://u:secret@h/db"}},
		// SF1: map, object, array and any-typed defaults, walked recursively.
		{"object", map[string]any{"dsn": "postgres://u:secret@h/db"}},
		{"json", map[string]any{"password": "secret"}},
		{"array", []any{map[string]any{"dsn": "postgres://u:secret@h/db"}}},
		{"any", map[string]any{"outer": map[string]any{"inner": []any{"x", map[string]any{"token": "abc"}}}}},
		{"any", map[string]string{"api_key": "abc123"}},
		{"any", map[string]any{"postgres://u:secret@h/db": true}},
		{"any", struct {
			DSN string `json:"dsn"`
		}{"u:secret@tcp(h)/db"}},
	}
	for _, c := range refused {
		v := newQueryDef("SQL", "SELECT 1")
		v.Parameters = Parameters{{ID: "dsn", Type: c.typ, DefaultValue: c.dv}}
		err := v.Validate()
		if err == nil {
			t.Errorf("expected parameter default %#v (type %s) to be refused", c.dv, c.typ)
			continue
		}
		if !strings.Contains(err.Error(), "parameters[0].defaultValue") {
			t.Errorf("expected the error to name the parameter default, got: %v", err)
		}
	}

	allowed := []any{
		"postgres://u@h/db", "hello", "user@example.com", 42, 3.5, true, nil,
		map[string]any{"token_count": 5, "password": "", "user": "reader", "nested": []any{"a", 1}},
		[]any{"a", map[string]any{"page_token": "abc"}},
		make(chan int), // not JSON-encodable: nothing to screen, and the write itself refuses it
	}
	for _, dv := range allowed {
		v := newQueryDef("SQL", "SELECT 1")
		v.Parameters = Parameters{{ID: "p", Type: "any", DefaultValue: dv}}
		if err := v.Validate(); err != nil {
			t.Errorf("expected parameter default %#v to be allowed, got: %v", dv, err)
		}
	}
}

func TestQueryDef_Validate_ScreensHTTPQueryText(t *testing.T) {
	for _, text := range []string{
		"GET https://admin:secret@api.example.com/x",
		"GET https://api.example.com/x\nAuthorization: Bearer abc",
		"GET https://api.example.com/x?api_key=abc123",
		"POST https://api.example.com/login\nContent-Type: application/json\n\n{\"user\":\"u\",\"password\":\"secret\"}",
	} {
		v := newQueryDef("HTTP", text)
		err := v.Validate()
		if err == nil || !strings.Contains(err.Error(), "text") {
			t.Errorf("expected HTTP text %q to be refused naming the text field, got: %v", text, err)
		}
	}
	for _, text := range []string{
		"GET https://api.example.com/items?page_token=abc",
		"GET https://api.example.com/x\nAuthorization: Bearer {{token}}",
		"POST https://api.example.com/x\n\n{\"token_count\": 5}",
	} {
		if err := newQueryDef("HTTP", text).Validate(); err != nil {
			t.Errorf("expected HTTP text %q to be allowed, got: %v", text, err)
		}
	}
	// SQL text is code a user writes on purpose; it is not screened.
	if err := newQueryDef("SQL", "SELECT * FROM users WHERE password = 'x'").Validate(); err != nil {
		t.Errorf("expected SQL text not to be screened, got: %v", err)
	}
}

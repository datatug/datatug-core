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
	for _, value := range []string{
		"scott/tiger@orcl", "https://user:123/x@h", "?key=AIzaSyA", "password: secret", "password=@dmin",
		"https://gmail.googleapis.com/gmail/v1/users/me/messages?q=from:alice@example.com",
	} {
		if _, found := EmbeddedCredentialReason(value); found {
			t.Errorf("%q is documented as not detected; update the package doc if that changed", value)
		}
	}
	for _, value := range []string{
		"SELECT * FROM users WHERE password = 'x'",
		"UPDATE users SET password = crypt(:pw, gen_salt('bf'))",
		"{\n  user(id: 1) {\n    authorization: permissions { read }\n  }\n}",
		"from:alice@example.com",
		"https://h/x?csrf_token=abc123",
		"a,b:c@d",
		"SELECT 'a:b@c'",
	} {
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
}

// Review SF-C: datatug-cli's HTTP executor substitutes a declared parameter
// referenced as {name} (pkg/httpsource/params.go). Referencing a secret
// that way keeps it out of git, so it must be allowed, while the same
// positions holding a literal are still refused - and the refusal must
// suggest the {name} syntax the executor substitutes, not {{name}}.
func TestQueryDef_Validate_AllowsTheHTTPParameterPlaceholder(t *testing.T) {
	for _, text := range []string{
		"https://api.example.com/v1/latest?api_key={apiKey}",
		"https://api.example.com/v1/latest?access_token={token}",
		"GET https://h/x\nAuthorization: Bearer {token}",
		"GET https://h/x\nX-API-Key: {apiKey}",
		`{"token": "{token}"}`,
		// The demo project's own HTTP queries.
		"https://countriesnow.space/api/v0.1/countries/currency/q?country={name}",
		"https://api.frankfurter.dev/v1/latest?from=USD&to={to}",
	} {
		if reason, found := EmbeddedCredentialReason(text); found {
			t.Errorf("expected %q to be allowed, got refused: %s", text, reason)
		}
		if err := newQueryDef("HTTP", text).Validate(); err != nil {
			t.Errorf("expected HTTP text %q to be allowed, got: %v", text, err)
		}
	}
	for _, text := range []string{
		"https://api.example.com/v1/latest?api_key=abc123",
		"GET https://h/x\nAuthorization: Bearer abc123",
		"GET https://h/x\nX-API-Key: abc123",
		`{"token": "abc123"}`,
		"https://api.example.com/v1/latest?api_key={api key}", // not a parameter name
	} {
		err := newQueryDef("HTTP", text).Validate()
		if err == nil {
			t.Errorf("expected HTTP text %q to be refused", text)
			continue
		}
		if msg := err.Error(); !strings.Contains(msg, "{name}") || strings.Contains(msg, "{{") {
			t.Errorf("expected the refusal to suggest the {name} placeholder the executor substitutes, got: %v", err)
		}
	}
}

// Review V1: a query's title and its text are stored in git-tracked files
// (the text as the body sidecar), so QueryDef.Validate screens both,
// whatever the query type.
func TestQueryDef_Validate_ScreensTheTitleAndEveryQueryText(t *testing.T) {
	for _, title := range []string{
		"prod postgres://u:secret@h/db",
		"Server=h;Password=secret",
		"Authorization: Bearer abc123",
	} {
		v := newQueryDef("SQL", "SELECT 1")
		v.Title = title
		err := v.Validate()
		if err == nil || !strings.Contains(err.Error(), "title") {
			t.Errorf("expected title %q to be refused naming the title field, got: %v", title, err)
		}
	}
	for _, c := range []struct {
		typ  QueryType
		text string
	}{
		{"SQL", "SELECT * FROM dblink('host=h user=u password=secret', 'select 1') AS t(x int)"},
		{"SQL", "-- postgres://u:secret@h/db\nSELECT 1"},
		{"SQL", "SELECT * FROM users WHERE password = 'x'"},
		{"SQL", "SELECT * FROM t WHERE api_key = 'abc123'"},
		{"SQL", "SELECT * FROM t WHERE token = abc123"},
		{"DTQL", "from:\n  name: Invoice\n# postgres://u:secret@h/db"},
		{"GraphQL", "# Authorization: Bearer secret\n{ a }"},
		{"GraphQL", `{"query": "{ me }", "variables": {"token": "abc123"}}`},
		{"SQL", "-- X-API-Key: abc123\nSELECT 1"},
		{"SQL", "/* Authorization: Bearer abc123 */\nSELECT 1"},
		{"SQL", "/*\n * Authorization: Bearer abc123\n */\nSELECT 1"},
		{"DTQL", "// Authorization: Basic dXNlcjpwYXNz\nfrom:\n  name: Invoice"},
		{"HTTP", "https://u:secret@h/x"},
	} {
		err := newQueryDef(c.typ, c.text).Validate()
		if err == nil || !strings.Contains(err.Error(), "text") {
			t.Errorf("expected %s text %q to be refused naming the text field, got: %v", c.typ, c.text, err)
			continue
		}
		wantHint := "@name"
		if c.typ == QueryTypeHTTP {
			wantHint = "{name}"
		}
		if !strings.Contains(err.Error(), wantHint) {
			t.Errorf("expected the %s refusal to suggest %s, got: %v", c.typ, wantHint, err)
		}
	}
}

// Bind parameters and template references name a value instead of holding
// one, so query text that uses them is allowed on every query type.
func TestQueryDef_Validate_AllowsPlaceholdersInQueryText(t *testing.T) {
	for _, text := range []string{
		"SELECT * FROM users WHERE token = @token",
		"SELECT * FROM t WHERE api_key = @apiKey",
		"UPDATE users SET password = :pw WHERE id = :id",
		"UPDATE users SET password = ? WHERE id = ?",
		"UPDATE users SET password = $1 WHERE id = $2",
		"SELECT * FROM t WHERE token = {token}",
		"SELECT * FROM t WHERE token = {{token}}",
		`UPDATE users SET "password" = crypt(:pw, gen_salt('bf'))`, // the documented workaround
		"UPDATE users SET password_hash = crypt($1, gen_salt('bf'))",
		"SELECT password FROM users",
		"SELECT * FROM users WHERE password = ''",
		"SELECT * FROM users WHERE password IS NULL",
		"SELECT * FROM t WHERE email = 'bob@example.com'",
		"SELECT * FROM logs WHERE msg LIKE '%token=%'",
	} {
		if err := newQueryDef("SQL", text).Validate(); err != nil {
			t.Errorf("expected SQL %q to be allowed, got: %v", text, err)
		}
	}
	for _, c := range []struct {
		typ  QueryType
		text string
	}{
		{"GraphQL", "query($token: String!) { login(token: $token) { id } }"},
		{"GraphQL", "# Authorization: Bearer {token}\n{ me { id } }"},
		{"SQL", "/* Authorization: Bearer {{token}} */\nSELECT 1"},
		{"GraphQL", `{"query": "{ me }", "variables": {"token": "{{token}}"}}`},
		{"DTQL", "from:\n  name: User\nwhere:\n  op: ==\n  left:\n    field: Token\n  right:\n    param: token\n"},
	} {
		if err := newQueryDef(c.typ, c.text).Validate(); err != nil {
			t.Errorf("expected %s %q to be allowed, got: %v", c.typ, c.text, err)
		}
	}
}

// The demo project's titles and bodies (datatug-demo-projects,
// demo-project-1/queries) are ordinary content and must all pass. They are
// copied here so the check holds without that checkout; the filestore
// package's demo fixture test also re-saves the real files when the
// checkout is present.
func TestQueryDef_Validate_AllowsTheDemoProjectQueries(t *testing.T) {
	for _, c := range []struct {
		title string
		typ   QueryType
		text  string
	}{
		{"Customer invoices", "DTQL", "from:\n  name: Invoice\n  alias: i\ncolumns:\n  - field: InvoiceId\n  - field: InvoiceDate\n  - field: BillingCity\n  - field: BillingCountry\n  - field: Total\nwhere:\n  op: ==\n  left:\n    field: CustomerId\n  right:\n    param: CustomerId\norderBy:\n  - field: InvoiceDate\n    desc: true\n"},
		{"Customer purchases by genre", "SQL", "SELECT\n    g.Name AS GenreName,\n    COUNT(il.InvoiceLineId) AS TracksPurchased,\n    SUM(il.UnitPrice * il.Quantity) AS TotalSpent\nFROM InvoiceLine AS il\nINNER JOIN Invoice AS i ON i.InvoiceId = il.InvoiceId\nINNER JOIN Track AS t ON t.TrackId = il.TrackId\nINNER JOIN Genre AS g ON g.GenreId = t.GenreId\nWHERE i.CustomerId = @CustomerId\nGROUP BY g.Name\nORDER BY TotalSpent DESC\n"},
		{"Invoice lines", "SQL", "SELECT\n    il.InvoiceLineId,\n    t.Name AS TrackName,\n    il.UnitPrice,\n    il.Quantity,\n    (il.UnitPrice * il.Quantity) AS LineTotal\nFROM InvoiceLine AS il\nINNER JOIN Track AS t ON t.TrackId = il.TrackId\nWHERE il.InvoiceId = @InvoiceId\nORDER BY il.InvoiceLineId\n"},
		{"Country facts (currency by country name)", "HTTP", "https://countriesnow.space/api/v0.1/countries/currency/q?country={name}\n"},
		{"Exchange rate for the customer's currency", "HTTP", "https://api.frankfurter.dev/v1/latest?from=USD&to={to}\n"},
		{"Albums by title", "SQL", "SELECT al.AlbumId, al.Title AS AlbumTitle FROM Album AS al WHERE al.AlbumId = IFNULL(@AlbumId, al.AlbumId) AND al.ArtistId = IFNULL(@ArtistId, al.ArtistId) ORDER BY Title"},
		{"Tracks by title", "SQL", "SELECT t.* FROM Track AS t WHERE t.GenreId = IFNULL(t.GenreId, t.GenreId) AND t.AlbumId = IFNULL(@AlbumId, t.AlbumId)"},
	} {
		v := newQueryDef(c.typ, c.text)
		v.Title = c.title
		if err := v.Validate(); err != nil {
			t.Errorf("expected the demo query %q to be allowed, got: %v", c.title, err)
		}
	}
}

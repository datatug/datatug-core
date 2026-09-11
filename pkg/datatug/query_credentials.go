package datatug

import (
	"bytes"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Credential screening for git-tracked query definitions.
//
// A QueryDef is persisted to git-tracked project files, so no field that
// can carry a secret may hold one. QueryDefTarget.Validate screens a
// target's driver, catalog, protocol, host and username; QueryDef.Validate
// screens the query's title, its text whatever the query type (SQL,
// GraphQL, DTQL and HTTP alike: the text is the git-tracked body sidecar)
// and every parameter default (strings, and maps, objects and arrays
// walked recursively, whatever the parameter's declared type). Every save
// path - PutQuery, SaveQuery, CreateQuery, UpdateQuery and a project save -
// validates the QueryDef first, so each of them refuses such a write before
// anything reaches disk. All of them use EmbeddedCredentialReason, which
// refuses a secret in each of these syntaxes:
//
//   - URL userinfo with a password, "postgres://user:secret@host/db",
//     including a URL embedded in a longer value
//     ("jdbc:postgresql://user:secret@host/db"). This is decided by
//     net/url's User.Password(), so a percent-encoded password counts, an
//     empty password that is explicitly present ("postgres://u:@h/db")
//     counts, and "postgres://user@host/db" is allowed. When net/url cannot
//     parse the URL - an unencoded "/", "?" or "#" in the password makes
//     the rest of it an invalid port - a "user:password@" at the start of
//     the authority is refused anyway, whatever the password contains.
//   - DSN userinfo without a "//" authority, "user:secret@tcp(host)/db"
//     (go-sql-driver/mysql, Snowflake and similar). An ordinary "name@host"
//     is allowed, and so are "mailto:", "sip:", "sips:", "xmpp:" and "tel:"
//     URIs and an all-digit "user" ("10:30@home").
//   - A key/value pair whose key names a secret, with a value that is not
//     empty and not a placeholder, in a key/value connection string
//     ("Server=h;User Id=u;Password=secret;"), a URL query
//     ("https://h/api?api_key=abc") or a JSON object member
//     ({"password": "secret"}). Keys are compared case-insensitively,
//     ignoring "_", "-", "." and spaces: a key ending in password, passwd,
//     pwd, token, apikey, accountkey, sharedaccesskey,
//     sharedaccesssignature or secret, or a key that is exactly pass or
//     sig - covering Azure AccountKey=, SharedAccessKey= and
//     SharedAccessSignature=, SAS sig=, token=, access_token=,
//     motherduck_token=, api_key=, apikey= and client_secret=. A key
//     ending in page_token, next_token, continuation_token or sync_token
//     is a pagination cursor, not a secret, and is allowed.
//   - An HTTP credential header line with a literal value:
//     "Authorization: Bearer abc", "Proxy-Authorization:", "X-API-Key:",
//     "Api-Key:", "X-Auth-Token:" and "X-Access-Token:", also when the
//     line is a comment ("# ", "// ", "-- ", "/* " or " * " before the
//     header name), since a header pasted into a SQL or GraphQL comment
//     is stored in git all the same.
//
// A value is not a secret when it is empty or a placeholder: "?", "$1", a
// bind name such as ":new_password" or "@CustomerId" (the SQL Server and
// SQLite style the demo project's own SQL uses), an environment reference
// such as "$PGPASSWORD" or "${PGPASSWORD}", an HTTP query parameter
// reference such as "{token}" (the syntax datatug-cli's HTTP executor
// substitutes, see its pkg/httpsource/params.go), a template such as
// "{{token}}" or "<password>", or one of true, false, yes, no, on, off,
// null and none. So a username alone, "reset_password=true",
// "token_count=5" and SQL such as "UPDATE users SET password =
// :new_password", "WHERE token = @token", "WHERE api_key = $1" or "SELECT
// password_hash FROM users" are all allowed.
//
// Known limits, each a deliberate trade-off against false positives:
//
//   - Oracle's "scott/tiger@orcl" is not detected: it cannot be told apart
//     from "actions/checkout@v4" or "user/repo@v1.2.3".
//   - A URL password that starts with digits and is followed by an
//     unencoded "/", "?" or "#" ("https://user:123/x@h") parses as a valid
//     host:port and is not detected; it cannot be told apart from
//     "https://h:443/users/bob@example.com".
//   - A secret under a key this list does not name (for example Google's
//     "?key="), or in YAML "password: secret" form, is not detected.
//   - Some values that only look like credentials are refused. SQL that
//     compares a secret-named column with a literal ("WHERE password =
//     'x'") is refused on purpose: it would put that literal in git. SQL
//     that assigns a function call to one ("SET password = crypt(:pw,
//     gen_salt('bf'))") is refused too, because the value is read up to
//     the first separator ("crypt(:pw"); quote the column name (SET
//     "password" = ...) to write it. So are a "u:@h" URL with an
//     explicitly empty password and a "name:word@host" token inside a
//     comma- or semicolon-separated list ("a,b:c@d") or a SQL string
//     literal ("SELECT 'a:b@c'").
//   - A literal that has a placeholder's shape ("@dmin", ":secret",
//     "{token}", "$SECRET") is taken for a placeholder and not detected.
//   - A GraphQL alias named authorization on its own line
//     ("authorization: permissions { read }") reads as a credential header
//     and is refused; rename the alias.

// Refusal reasons returned by EmbeddedCredentialReason.
const (
	urlPasswordReason  = "must not embed a password in a URL (user:password@); a username alone is allowed"
	dsnPasswordReason  = "must not embed a password in a connection string (user:password@); a username alone is allowed"
	keyValueReason     = "must not embed a secret key/value pair (password=, pwd=, token=, api_key=, AccountKey=, sig=, ...) in a connection string"
	jsonSecretReason   = `must not embed a secret JSON member ("password": ..., "token": ...)`
	httpHeaderReason   = "must not embed an HTTP credential header (Authorization:, X-API-Key:, ...)"
	secretMapKeyReason = "must not hold a secret under the key "
)

var (
	// dsnUserinfoPattern matches a "user:password@" userinfo token that is
	// not part of a URL: group 2 containing "//" means group 1 was a URL
	// scheme ("postgres://u@h"), which urlCredentialReason decides instead.
	dsnUserinfoPattern = regexp.MustCompile(`(?:^|[\s;,(=])([^\s:@/;,()=]+):([^\s@;,]+)@`)

	// urlUserinfoPattern reads a "user:password@" userinfo at the start of
	// a URL's authority by hand, for a URL net/url refused: the username
	// stops at ":", "/", "?", "#" or "@"; the password runs to the first
	// "@", whatever else it contains.
	urlUserinfoPattern = regexp.MustCompile(`^[^\s:/?#@]*:[^\s@]*@`)

	// keyValuePattern matches "key=" after a separator; the value that
	// follows is read by keyValueValue.
	keyValuePattern = regexp.MustCompile(`(?:^|[;&?,\s{('"])([A-Za-z0-9_.\-]+)[ \t]*=[ \t]*`)

	// jsonMemberPattern matches a JSON object member with a string value.
	jsonMemberPattern = regexp.MustCompile(`"([^"\\]{1,128})"\s*:\s*"((?:[^"\\]|\\.)*)"`)

	// httpCredentialHeaderPattern matches an HTTP header line that carries
	// a credential, optionally inside a comment (#, //, --, /* or *).
	httpCredentialHeaderPattern = regexp.MustCompile(`(?im)^[ \t]*(?:(?:#|//|--|/\*|\*)[ \t]*)?(authorization|proxy-authorization|x-api-key|api-key|x-auth-token|x-access-token)[ \t]*:[ \t]*([^\r\n]*)`)

	// placeholderValuePattern matches a value that names a secret instead
	// of holding one.
	// "{name}" is the HTTP executor's parameter syntax, matched exactly as
	// datatug-cli's pkg/httpsource/params.go matches it; "@name" and
	// ":name" are SQL bind parameters.
	placeholderValuePattern = regexp.MustCompile(`^(?:\?|\$\d+|:[A-Za-z_]\w*|@[A-Za-z_]\w*|\$[A-Z_][A-Z0-9_]*|\$\{[^{}]*\}|\{[A-Za-z0-9_]+\}|\{\{[^{}]*\}\}|<[^<>]*>)$`)
)

var (
	secretKeyNames       = map[string]bool{"pass": true, "sig": true}
	secretKeySuffixes    = []string{"password", "passwd", "pwd", "token", "apikey", "accountkey", "sharedaccesskey", "sharedaccesssignature", "secret"}
	nonSecretKeySuffixes = []string{"pagetoken", "nexttoken", "continuationtoken", "synctoken"}
	authoritylessSchemes = map[string]bool{"mailto": true, "sip": true, "sips": true, "xmpp": true, "tel": true}
	httpAuthSchemes      = map[string]bool{"basic": true, "bearer": true, "digest": true, "token": true, "negotiate": true, "ntlm": true, "apikey": true, "aws4-hmac-sha256": true}
	nonSecretWords       = map[string]bool{"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true, "null": true, "none": true}
)

// EmbeddedCredentialReason reports why value appears to embed a secret -
// a password, token or key - in one of the syntaxes the package's
// credential-screening notes list (a URL or DSN userinfo, a secret
// key/value pair or JSON member, an HTTP credential header), or
// ("", false) when it does not. A username alone is never refused. It is
// the one screening function for anything that is persisted to
// git-tracked project files; QueryDefTarget.Validate and QueryDef.Validate
// use it, and so can any other caller that must keep secrets out of a
// project.
func EmbeddedCredentialReason(value string) (reason string, found bool) {
	for _, check := range [...]func(string) (string, bool){
		urlCredentialReason,
		dsnCredentialReason,
		keyValueCredentialReason,
		jsonCredentialReason,
		httpHeaderCredentialReason,
	} {
		if reason, found := check(value); found {
			return reason, true
		}
	}
	return "", false
}

// urlCredentialReason checks every "scheme://" URL embedded in value.
func urlCredentialReason(value string) (reason string, found bool) {
	for offset := 0; ; {
		i := strings.Index(value[offset:], "://")
		if i < 0 {
			return "", false
		}
		sep := offset + i
		offset = sep + len("://")

		start := sep
		for start > 0 && isURLSchemeByte(value[start-1]) {
			start--
		}
		end := len(value)
		if j := strings.IndexAny(value[offset:], " \t\r\n"); j >= 0 {
			end = offset + j
		}
		if u, err := url.Parse(value[start:end]); err == nil {
			if u.User != nil {
				if _, hasPassword := u.User.Password(); hasPassword {
					return urlPasswordReason, true
				}
			}
			continue
		}
		// net/url refused the candidate (an unencoded "/", "?" or "#" in
		// the password, a stray character in the host, an invalid escape):
		// read the userinfo by hand, so a value that cannot be parsed still
		// cannot smuggle a password past this check.
		if urlUserinfoPattern.MatchString(value[offset:end]) {
			return urlPasswordReason, true
		}
	}
}

func isURLSchemeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// dsnCredentialReason checks for a "user:password@" outside a URL.
func dsnCredentialReason(value string) (reason string, found bool) {
	for _, m := range dsnUserinfoPattern.FindAllStringSubmatch(value, -1) {
		user, password := m[1], m[2]
		if strings.Contains(password, "//") || authoritylessSchemes[strings.ToLower(user)] || isAllDigits(user) {
			continue
		}
		return dsnPasswordReason, true
	}
	return "", false
}

func isAllDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// keyValueCredentialReason checks every "key=value" pair in value.
func keyValueCredentialReason(value string) (reason string, found bool) {
	for _, m := range keyValuePattern.FindAllStringSubmatchIndex(value, -1) {
		key := value[m[2]:m[3]]
		if isSecretKey(key) && !isNonSecretValue(keyValueValue(value[m[1]:])) {
			return keyValueReason, true
		}
	}
	return "", false
}

// keyValueValue returns the value at the start of s: a quoted value up to
// its closing quote, otherwise up to the next separator.
func keyValueValue(s string) string {
	if s != "" && (s[0] == '\'' || s[0] == '"') {
		if end := strings.IndexByte(s[1:], s[0]); end >= 0 {
			return s[1 : 1+end]
		}
		return s[1:]
	}
	if end := strings.IndexAny(s, ";&,) \t\r\n"); end >= 0 {
		return s[:end]
	}
	return s
}

// jsonCredentialReason checks every JSON object member with a string value.
func jsonCredentialReason(value string) (reason string, found bool) {
	for _, m := range jsonMemberPattern.FindAllStringSubmatch(value, -1) {
		if isSecretKey(m[1]) && !isNonSecretValue(m[2]) {
			return jsonSecretReason, true
		}
	}
	return "", false
}

// httpHeaderCredentialReason checks every HTTP credential header line.
func httpHeaderCredentialReason(value string) (reason string, found bool) {
	for _, m := range httpCredentialHeaderPattern.FindAllStringSubmatch(value, -1) {
		// A block comment may close on the header's own line.
		header, credential := strings.ToLower(m[1]), strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m[2]), "*/"))
		if header == "authorization" || header == "proxy-authorization" {
			if fields := strings.Fields(credential); len(fields) > 0 && httpAuthSchemes[strings.ToLower(fields[0])] {
				credential = strings.Join(fields[1:], " ")
			}
		}
		if !isNonSecretValue(credential) {
			return httpHeaderReason, true
		}
	}
	return "", false
}

// isSecretKey reports whether key names a secret (see the package's
// credential-screening notes).
func isSecretKey(key string) bool {
	k := strings.Map(func(r rune) rune {
		switch r {
		case '_', '-', '.', ' ':
			return -1
		}
		return r
	}, strings.ToLower(key))
	if secretKeyNames[k] {
		return true
	}
	for _, suffix := range nonSecretKeySuffixes {
		if strings.HasSuffix(k, suffix) {
			return false
		}
	}
	for _, suffix := range secretKeySuffixes {
		if strings.HasSuffix(k, suffix) {
			return true
		}
	}
	return false
}

// isNonSecretValue reports whether v is empty or a placeholder rather than
// a secret.
func isNonSecretValue(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || nonSecretWords[strings.ToLower(v)] || placeholderValuePattern.MatchString(v)
}

// defaultValueCredentialReason screens a parameter default. A string is
// screened with EmbeddedCredentialReason; any other value is screened in
// the JSON form it is persisted in, recursively: every string, every map
// key, and every string held under a secret key (see isSecretKey).
func defaultValueCredentialReason(v any) (reason string, found bool) {
	switch v := v.(type) {
	case nil:
		return "", false
	case string:
		return EmbeddedCredentialReason(v)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // screen "&" as "&", not "\u0026"
	if err := enc.Encode(v); err != nil {
		return "", false // not persistable either; the write reports that
	}
	var decoded any
	_ = json.Unmarshal(buf.Bytes(), &decoded) // cannot fail on the encoder's own output
	return jsonValueCredentialReason(decoded)
}

// jsonValueCredentialReason walks a decoded JSON value (see
// defaultValueCredentialReason). Map keys are visited in sorted order so
// the reason reported is deterministic.
func jsonValueCredentialReason(v any) (reason string, found bool) {
	switch v := v.(type) {
	case string:
		return EmbeddedCredentialReason(v)
	case []any:
		for _, item := range v {
			if reason, found := jsonValueCredentialReason(item); found {
				return reason, true
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if s, isString := v[k].(string); isString && isSecretKey(k) && !isNonSecretValue(s) {
				return secretMapKeyReason + strconv.Quote(k), true
			}
			if reason, found := EmbeddedCredentialReason(k); found {
				return reason, true
			}
			if reason, found := jsonValueCredentialReason(v[k]); found {
				return reason, true
			}
		}
	}
	return "", false
}

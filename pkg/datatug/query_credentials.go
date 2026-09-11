package datatug

import (
	"net/url"
	"regexp"
	"strings"
)

// Credential screening for git-tracked query definitions.
//
// A QueryDef is persisted to git-tracked project files, so no field that
// can carry a connection string may carry a secret. A username alone may
// identify a connection and is allowed. A password is refused in each of
// the common connection-string syntaxes:
//
//   - URL userinfo with a password, "postgres://user:secret@host/db",
//     including a URL embedded in a longer value
//     ("jdbc:postgresql://user:secret@host/db"). This is decided by
//     net/url's User.Password(), so a percent-encoded password counts, an
//     empty password that is explicitly present counts, and
//     "postgres://user@host/db" is allowed.
//   - DSN userinfo without a "//" authority, "user:secret@tcp(host)/db"
//     (go-sql-driver/mysql, Snowflake and similar). Any "token:token@" that
//     is not part of a URL is read this way, which also refuses a
//     "mailto:name@host" value; an ordinary "name@host" is allowed.
//   - A key/value pair whose key ends in password, passwd or pwd with a
//     non-empty value, case-insensitively, in a key/value connection
//     string ("Server=h;User Id=u;Password=secret;") or a URL query
//     ("postgres://u@h/db?password=secret"). An empty value ("Password=;")
//     holds no secret and is allowed.

var (
	// dsnUserinfoPattern matches a "user:password@" userinfo token that is
	// not part of a URL: group 2 containing "//" means group 1 was a URL
	// scheme ("postgres://u@h"), which urlCredentialReason decides instead.
	dsnUserinfoPattern = regexp.MustCompile(`(?:^|[\s;,(=])([^\s:@/;,()=]+):([^\s@;,]+)@`)

	// keyValuePasswordPattern matches a password-like key followed by "="
	// and a non-empty value.
	keyValuePasswordPattern = regexp.MustCompile(`(?i)(?:^|[;&?,\s])[\w.-]*(?:password|passwd|pwd)\s*=\s*[^;&\s]`)
)

// embeddedCredentialReason reports why value appears to embed a password in
// one of the connection-string syntaxes documented above, or ("", false)
// when it does not.
func embeddedCredentialReason(value string) (reason string, found bool) {
	if reason, found := urlCredentialReason(value); found {
		return reason, true
	}
	for _, m := range dsnUserinfoPattern.FindAllStringSubmatch(value, -1) {
		if !strings.Contains(m[2], "//") {
			return "must not embed a password in a connection string (user:password@); a username alone is allowed", true
		}
	}
	if keyValuePasswordPattern.MatchString(value) {
		return "must not embed a password key (password=, pwd=) in a connection string", true
	}
	return "", false
}

// urlCredentialReason checks every "scheme://" URL embedded in value.
func urlCredentialReason(value string) (reason string, found bool) {
	const refused = "must not embed a password in a URL (user:password@); a username alone is allowed"
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
		if u, err := url.Parse(value[start:end]); err == nil && u.User != nil {
			if _, hasPassword := u.User.Password(); hasPassword {
				return refused, true
			}
			continue
		}
		// net/url refused the candidate (a stray character in the host, an
		// invalid escape, no scheme letter): fall back to reading the
		// authority by hand, so a value that cannot be parsed still cannot
		// smuggle a password past this check.
		authority := value[offset:end]
		if j := strings.IndexAny(authority, "/?#;"); j >= 0 {
			authority = authority[:j]
		}
		if at := strings.LastIndexByte(authority, '@'); at >= 0 && strings.ContainsRune(authority[:at], ':') {
			return refused, true
		}
	}
}

func isURLSchemeByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'
}

// defaultValueCredentialReason applies embeddedCredentialReason to a
// parameter default: a string, or each string of a multi-value default.
func defaultValueCredentialReason(v any) (reason string, found bool) {
	switch v := v.(type) {
	case string:
		return embeddedCredentialReason(v)
	case []string:
		for _, s := range v {
			if reason, found := embeddedCredentialReason(s); found {
				return reason, true
			}
		}
	case []any:
		for _, item := range v {
			if reason, found := defaultValueCredentialReason(item); found {
				return reason, true
			}
		}
	}
	return "", false
}

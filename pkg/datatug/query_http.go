package datatug

import (
	"fmt"
	"strings"

	"github.com/strongo/validation"
)

const (
	// MaxQueryHTTPBodyBytes bounds a saved HTTP query body.
	MaxQueryHTTPBodyBytes = 256 * 1024
	// MaxQueryHTTPHeaders bounds the saved headers of an HTTP query.
	MaxQueryHTTPHeaders = 50
)

// QueryHTTPRequest is the saved request of an HTTP query beyond its URL, which
// stays in QueryDef.Text. It never holds credentials.
type QueryHTTPRequest struct {
	Method  string           `json:"method,omitempty" yaml:"method,omitempty" firestore:"method,omitempty"`
	Headers []HTTPHeaderItem `json:"headers,omitempty" yaml:"headers,omitempty" firestore:"headers,omitempty"`
	Body    string           `json:"body,omitempty" yaml:"body,omitempty" firestore:"body,omitempty"`
}

// IsCredentialHTTPHeader reports whether a header name carries credentials and
// so must never be saved with a query. Matching is case-insensitive.
func IsCredentialHTTPHeader(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	switch n {
	case "authorization", "proxy-authorization", "cookie", "set-cookie",
		"x-api-key", "x-auth-token", "x-access-token", "x-amz-security-token":
		return true
	}
	for _, part := range []string{"token", "secret", "password", "api-key", "apikey"} {
		if strings.Contains(n, part) {
			return true
		}
	}
	return false
}

// IsHTTPToken reports whether s is a non-empty RFC 7230 token.
func IsHTTPToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// HTTPMethodAllowsBody reports whether the method (empty = GET) may carry a body.
func HTTPMethodAllowsBody(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	}
	return false
}

// Validate returns error if not valid. A nil request is valid (plain GET).
func (v *QueryHTTPRequest) Validate() error {
	if v == nil {
		return nil
	}
	method := v.Method
	switch method {
	case "":
	case "GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS":
	default:
		return validation.NewErrBadRecordFieldValue("method", "must be one of GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS")
	}
	if len(v.Headers) > MaxQueryHTTPHeaders {
		return validation.NewErrBadRecordFieldValue("headers", fmt.Sprintf("at most %d headers are allowed", MaxQueryHTTPHeaders))
	}
	for i, h := range v.Headers {
		field := fmt.Sprintf("headers[%d]", i)
		if !IsHTTPToken(h.Name) {
			return validation.NewErrBadRecordFieldValue(field+".name", "must be a valid HTTP header name")
		}
		if strings.ContainsAny(h.Value, "\r\n") {
			return validation.NewErrBadRecordFieldValue(field+".value", "must not contain line breaks")
		}
		if reason, found := embeddedCredentialReasonWithoutDSN(h.Value); found {
			return validation.NewErrBadRecordFieldValue(field+".value", reason+"; set credentials for a single run instead of saving them")
		}
		if IsCredentialHTTPHeader(h.Name) {
			return validation.NewErrBadRecordFieldValue(field+".name", "credential headers are never saved with a query; set them for a single run instead")
		}
	}
	if v.Body != "" {
		if !HTTPMethodAllowsBody(method) {
			return validation.NewErrBadRecordFieldValue("body", "is only allowed for POST, PUT, PATCH and DELETE")
		}
		if len(v.Body) > MaxQueryHTTPBodyBytes {
			return validation.NewErrBadRecordFieldValue("body", fmt.Sprintf("is over the %d-byte limit", MaxQueryHTTPBodyBytes))
		}
		if reason, found := embeddedCredentialReasonWithoutDSN(v.Body); found {
			return validation.NewErrBadRecordFieldValue("body", reason+"; reference a declared query parameter as {name} instead of a literal secret")
		}
	}
	return nil
}

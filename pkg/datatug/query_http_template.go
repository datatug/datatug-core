package datatug

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// HTTP query parameters (datatug-apps spec/features/http-query-parameters).
//
// An HTTP query's URL, header values and body are templates: {name} is a
// placeholder for the declared parameter `name`. This file is the Go half of
// the one substituter shared with the web app (datatug-apps
// libs/datatug/main/src/lib/queries/http-template.ts); both run the same test
// vectors (testdata/http_template_vectors.json, copied verbatim into the web
// app), so the web browser, the DataTug server and save validation agree.
//
// Decisions recorded with the spec's open questions (founder-approved,
// 2026-10-22):
//   - one run binds one row: there is no per-row fan-out;
//   - the CLI keeps running only the URL as a GET; header and body
//     placeholders are web-only;
//   - there is no brace escape: `{{name}}` is literal text around a
//     placeholder and JSON objects (`{"`) never match;
//   - placeholders are forbidden in the host (and scheme, userinfo, port and
//     fragment), so a saved query is pinned to the origin its template names
//     - no separate per-query origin allowlist is needed.
//
// One stricter-than-spec rule: a Content-Type header value may not hold a
// placeholder, because the body's encoding mode is decided from it before
// substitution.

// httpPlaceholderRe is the placeholder expression of dalgo2http
// (descriptor.go) and datatug-cli (pkg/httpsource/params.go).
var httpPlaceholderRe = regexp.MustCompile(`\{([a-zA-Z0-9_]+)\}`)

// httpPlaceholderPrefixRe matches a placeholder at the start of the input only.
var httpPlaceholderPrefixRe = regexp.MustCompile(`^\{([a-zA-Z0-9_]+)\}`)

var (
	httpParamIDRe = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)
	httpSchemeRe  = regexp.MustCompile(`^[hH][tT][tT][pP][sS]?$`)
	// httpHostRe is a non-empty host name or IP literal with an optional port.
	httpHostRe = regexp.MustCompile(`^(?:\[[0-9A-Fa-f:.]+\]|[A-Za-z0-9][A-Za-z0-9._~-]*)(?::[0-9]{1,5})?$`)
)

// HTTPPosition names where a placeholder sits in an HTTP request template.
type HTTPPosition string

// Positions of a placeholder. Only path, query, header, body, json-string and
// json-value are allowed; the others are reported by forbidden-position errors.
const (
	HTTPPositionMethod      HTTPPosition = "method"
	HTTPPositionScheme      HTTPPosition = "scheme"
	HTTPPositionUserinfo    HTTPPosition = "userinfo"
	HTTPPositionHost        HTTPPosition = "host"
	HTTPPositionPath        HTTPPosition = "path"
	HTTPPositionQueryName   HTTPPosition = "query-name"
	HTTPPositionQuery       HTTPPosition = "query"
	HTTPPositionFragment    HTTPPosition = "fragment"
	HTTPPositionHeaderName  HTTPPosition = "header-name"
	HTTPPositionHeader      HTTPPosition = "header"
	HTTPPositionContentType HTTPPosition = "content-type"
	// HTTPPositionReservedHeader is the value of a header the transport owns
	// or that routes the request (Host, Content-Length, Forwarded, ...).
	HTTPPositionReservedHeader HTTPPosition = "reserved-header"
	HTTPPositionBody           HTTPPosition = "body"
	HTTPPositionJSONString     HTTPPosition = "json-string"
	HTTPPositionJSONValue      HTTPPosition = "json-value"
	HTTPPositionJSONKey        HTTPPosition = "json-key"
	HTTPPositionJSONOther      HTTPPosition = "json-other"
)

// HTTPTemplateErrorCode classifies an HTTPTemplateError; the codes are shared
// with the web app.
type HTTPTemplateErrorCode string

// Error codes of HTTPTemplateError.
const (
	HTTPErrInvalidURL         HTTPTemplateErrorCode = "invalid-url"
	HTTPErrForbiddenPosition  HTTPTemplateErrorCode = "forbidden-position"
	HTTPErrInvalidJSON        HTTPTemplateErrorCode = "invalid-json"
	HTTPErrBodyNotAllowed     HTTPTemplateErrorCode = "body-not-allowed"
	HTTPErrInvalidDeclaration HTTPTemplateErrorCode = "invalid-declaration"
	HTTPErrDuplicateParameter HTTPTemplateErrorCode = "duplicate-parameter"
	HTTPErrUndeclared         HTTPTemplateErrorCode = "undeclared"
	HTTPErrUnused             HTTPTemplateErrorCode = "unused"
	HTTPErrDefaultRequired    HTTPTemplateErrorCode = "default-required"
	HTTPErrCredential         HTTPTemplateErrorCode = "credential-parameter"
	HTTPErrJSONType           HTTPTemplateErrorCode = "json-type"
	HTTPErrMissingValue       HTTPTemplateErrorCode = "missing-value"
	HTTPErrInvalidValue       HTTPTemplateErrorCode = "invalid-value"
	HTTPErrDotSegment         HTTPTemplateErrorCode = "dot-segment"
	HTTPErrHeaderLineBreak    HTTPTemplateErrorCode = "header-line-break"
	HTTPErrContentType        HTTPTemplateErrorCode = "duplicate-content-type"
	HTTPErrTooLarge           HTTPTemplateErrorCode = "too-large"
)

// HTTPTemplateError is why a template cannot be saved or run. It never holds
// a parameter value.
type HTTPTemplateError struct {
	Code      HTTPTemplateErrorCode `json:"code"`
	Parameter string                `json:"parameter,omitempty"`
	Position  HTTPPosition          `json:"position,omitempty"`
	Header    string                `json:"header,omitempty"`
	Detail    string                `json:"detail"`
}

func (e *HTTPTemplateError) Error() string {
	var b strings.Builder
	b.WriteString("http template")
	if e.Parameter != "" {
		b.WriteString(": parameter " + e.Parameter)
	}
	if e.Header != "" {
		b.WriteString(": header " + e.Header)
	}
	if e.Position != "" {
		b.WriteString(": position " + string(e.Position))
	}
	b.WriteString(": " + e.Detail)
	return b.String()
}

// Size caps of a substituted request: the body shares the saved-body limit,
// and the URL and each header value are capped so a large value repeated by
// many placeholders cannot exhaust memory.
const (
	MaxHTTPTemplateURLBytes    = 64 * 1024
	MaxHTTPTemplateHeaderBytes = 64 * 1024
)

// reservedHTTPHeaders are headers whose value cannot hold a placeholder: the
// transport owns them or they route the request.
var reservedHTTPHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true, "connection": true,
	"keep-alive": true, "upgrade": true, "te": true, "trailer": true, "forwarded": true,
	"proxy-connection": true, "expect": true, "via": true, "x-real-ip": true,
	"x-original-url": true, "x-rewrite-url": true, "x-host": true,
}

func isReservedHTTPHeader(name string) bool {
	n := strings.ToLower(asciiTrim(name))
	return reservedHTTPHeaders[n] || strings.HasPrefix(n, "proxy-") || strings.HasPrefix(n, "x-forwarded-")
}

// asciiTrim trims ASCII whitespace only, so Go and JavaScript agree.
func asciiTrim(s string) string {
	return strings.Trim(s, " \t\r\n")
}

// HTTPParameterTypes are the parameter types an HTTP query may declare.
var HTTPParameterTypes = []string{"string", "integer", "number", "boolean", "bit", "date", "datetime"}

// HTTPRequestTemplate is an HTTP request whose URL, header values and body may
// hold {name} placeholders.
type HTTPRequestTemplate struct {
	Method  string           `json:"method,omitempty"`
	URL     string           `json:"url"`
	Headers []HTTPHeaderItem `json:"headers,omitempty"`
	Body    string           `json:"body,omitempty"`
}

// HTTPPlaceholder is one placeholder occurrence in a template.
type HTTPPlaceholder struct {
	Name     string       `json:"name"`
	Position HTTPPosition `json:"position"`
	// Header is the header name for a header-value placeholder.
	Header string `json:"header,omitempty"`
	// Credential is true for a placeholder in a credential header value
	// (IsCredentialHTTPHeader).
	Credential bool `json:"credential,omitempty"`
}

type httpField int

const (
	httpFieldURL httpField = iota
	httpFieldHeader
	httpFieldBody
)

// httpSpan is a placeholder occurrence with its byte range in its field.
type httpSpan struct {
	HTTPPlaceholder
	field      httpField
	header     int // header index for httpFieldHeader
	start, end int
}

func forbidden(name string, pos HTTPPosition, header, detail string) *HTTPTemplateError {
	return &HTTPTemplateError{Code: HTTPErrForbiddenPosition, Parameter: name, Position: pos, Header: header, Detail: detail}
}

// isLiteralBraces reports whether the match s[start:end] is wrapped as
// {{name}}: that is literal text, not a placeholder (REQ:placeholder-syntax).
func isLiteralBraces(s string, start, end int) bool {
	return start > 0 && s[start-1] == '{' && end < len(s) && s[end] == '}'
}

// placeholderMatches returns the submatch indexes of every placeholder in s.
func placeholderMatches(s string) [][]int {
	var out [][]int
	for _, m := range httpPlaceholderRe.FindAllStringSubmatchIndex(s, -1) {
		if !isLiteralBraces(s, m[0], m[1]) {
			out = append(out, m)
		}
	}
	return out
}

// firstPlaceholder returns the first placeholder name in s, or "".
func firstPlaceholder(s string) string {
	if m := placeholderMatches(s); m != nil {
		return s[m[0][2]:m[0][3]]
	}
	return ""
}

// HTTPTemplateHasPlaceholders reports whether the template holds any {name}
// placeholder.
func HTTPTemplateHasPlaceholders(t HTTPRequestTemplate) bool {
	if firstPlaceholder(t.URL) != "" || firstPlaceholder(t.Method) != "" || firstPlaceholder(t.Body) != "" {
		return true
	}
	for _, h := range t.Headers {
		if firstPlaceholder(h.Name) != "" || firstPlaceholder(h.Value) != "" {
			return true
		}
	}
	return false
}

// Placeholders lists every placeholder of the template in order: URL,
// headers, body. It fails on a placeholder in a forbidden position, on a
// body placeholder for a method without a body and on invalid JSON around
// placeholders in a JSON body.
func (t HTTPRequestTemplate) Placeholders() ([]HTTPPlaceholder, error) {
	spans, err := t.spans()
	if err != nil {
		return nil, err
	}
	out := make([]HTTPPlaceholder, len(spans))
	for i, s := range spans {
		out[i] = s.HTTPPlaceholder
	}
	return out, nil
}

func (t HTTPRequestTemplate) spans() ([]httpSpan, error) {
	if name := firstPlaceholder(t.Method); name != "" {
		return nil, forbidden(name, HTTPPositionMethod, "", "the method cannot hold a placeholder")
	}
	spans, err := urlSpans(asciiTrim(t.URL))
	if err != nil {
		return nil, err
	}
	for i, h := range t.Headers {
		if name := firstPlaceholder(h.Name); name != "" {
			return nil, forbidden(name, HTTPPositionHeaderName, h.Name, "a header name cannot hold a placeholder")
		}
		credential := IsCredentialHTTPHeader(h.Name)
		for _, m := range placeholderMatches(h.Value) {
			name := h.Value[m[2]:m[3]]
			if isContentTypeHeader(h.Name) {
				return nil, forbidden(name, HTTPPositionContentType, h.Name, "the Content-Type header decides how the body is encoded, so it cannot hold a placeholder")
			}
			if isReservedHTTPHeader(h.Name) {
				return nil, forbidden(name, HTTPPositionReservedHeader, h.Name, "this header routes the request or belongs to the transport, so it cannot hold a placeholder")
			}
			spans = append(spans, httpSpan{
				HTTPPlaceholder: HTTPPlaceholder{Name: name, Position: HTTPPositionHeader, Header: h.Name, Credential: credential},
				field:           httpFieldHeader, header: i, start: m[0], end: m[1],
			})
		}
	}
	if name := firstPlaceholder(t.Body); name != "" {
		method := t.Method
		if method == "" {
			method = "GET"
		}
		if !HTTPMethodAllowsBody(method) {
			return nil, &HTTPTemplateError{Code: HTTPErrBodyNotAllowed, Parameter: name, Position: HTTPPositionBody,
				Detail: "only POST, PUT, PATCH and DELETE requests carry a body, so the body cannot hold a placeholder"}
		}
		contentTypes := 0
		for _, h := range t.Headers {
			if isContentTypeHeader(h.Name) {
				contentTypes++
			}
		}
		if contentTypes > 1 {
			return nil, &HTTPTemplateError{Code: HTTPErrContentType, Parameter: name, Position: HTTPPositionBody,
				Detail: "a body with placeholders needs exactly one Content-Type header, which decides how values are encoded"}
		}
		var bodySpans []httpSpan
		switch httpBodyMode(t.Headers) {
		case httpBodyJSON:
			bodySpans, err = jsonBodySpans(t.Body)
			if err != nil {
				return nil, err
			}
		default:
			for _, m := range placeholderMatches(t.Body) {
				bodySpans = append(bodySpans, httpSpan{
					HTTPPlaceholder: HTTPPlaceholder{Name: t.Body[m[2]:m[3]], Position: HTTPPositionBody},
					start:           m[0], end: m[1],
				})
			}
		}
		for i := range bodySpans {
			bodySpans[i].field = httpFieldBody
		}
		spans = append(spans, bodySpans...)
	}
	return spans, nil
}

// urlSpans classifies the placeholders of a URL template. The path/query
// split is at the first '?' after the authority, as in datatug-cli.
func urlSpans(u string) ([]httpSpan, error) {
	if firstPlaceholder(u) == "" {
		return nil, nil
	}
	rest := u
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		if name := firstPlaceholder(rest[i+1:]); name != "" {
			return nil, forbidden(name, HTTPPositionFragment, "", "the URL fragment cannot hold a placeholder")
		}
		rest = rest[:i]
	}
	schemeEnd := strings.Index(rest, "://")
	if schemeEnd >= 0 {
		if name := firstPlaceholder(rest[:schemeEnd]); name != "" {
			return nil, forbidden(name, HTTPPositionScheme, "", "the URL scheme cannot hold a placeholder")
		}
	}
	if schemeEnd < 0 || !httpSchemeRe.MatchString(rest[:schemeEnd]) {
		return nil, &HTTPTemplateError{Code: HTTPErrInvalidURL, Detail: "a URL with placeholders must be an absolute http or https URL, such as https://host/path"}
	}
	// Browsers (WHATWG URL) read a backslash as a slash and drop tabs and
	// line breaks, so such a URL could resolve to another host than the one
	// checked here.
	if strings.IndexFunc(u, func(r rune) bool { return r == '\\' || r <= ' ' || r == 0x7f }) >= 0 {
		return nil, &HTTPTemplateError{Code: HTTPErrInvalidURL, Detail: "a URL with placeholders cannot contain spaces, control characters or backslashes"}
	}
	offset := schemeEnd + 3
	afterScheme := rest[offset:]
	authority, pathAndQuery := afterScheme, ""
	if i := strings.IndexAny(afterScheme, "/?"); i >= 0 {
		authority, pathAndQuery = afterScheme[:i], afterScheme[i:]
	}
	if at := strings.LastIndexByte(authority, '@'); at >= 0 {
		if name := firstPlaceholder(authority[:at]); name != "" {
			return nil, forbidden(name, HTTPPositionUserinfo, "", "the URL userinfo cannot hold a placeholder")
		}
		authority = authority[at+1:]
	}
	if name := firstPlaceholder(authority); name != "" {
		return nil, forbidden(name, HTTPPositionHost, "", "the URL host and port cannot hold a placeholder: a saved query is pinned to its origin")
	}
	if !httpHostRe.MatchString(authority) {
		return nil, &HTTPTemplateError{Code: HTTPErrInvalidURL, Detail: "a URL with placeholders must name its host"}
	}
	offset += len(afterScheme) - len(pathAndQuery)
	path, query := pathAndQuery, ""
	queryAt := strings.IndexByte(pathAndQuery, '?')
	if queryAt >= 0 {
		path, query = pathAndQuery[:queryAt], pathAndQuery[queryAt+1:]
	}
	var spans []httpSpan
	for _, m := range placeholderMatches(path) {
		spans = append(spans, httpSpan{
			HTTPPlaceholder: HTTPPlaceholder{Name: path[m[2]:m[3]], Position: HTTPPositionPath},
			start:           offset + m[0], end: offset + m[1],
		})
	}
	if queryAt < 0 {
		return spans, nil
	}
	pairStart := offset + queryAt + 1
	for _, pair := range strings.Split(query, "&") {
		name, value, hasValue := strings.Cut(pair, "=")
		if p := firstPlaceholder(name); p != "" {
			return nil, forbidden(p, HTTPPositionQueryName, "", "a query-string name cannot hold a placeholder")
		}
		if hasValue {
			valueStart := pairStart + len(name) + 1
			for _, m := range placeholderMatches(value) {
				spans = append(spans, httpSpan{
					HTTPPlaceholder: HTTPPlaceholder{Name: value[m[2]:m[3]], Position: HTTPPositionQuery},
					start:           valueStart + m[0], end: valueStart + m[1],
				})
			}
		}
		pairStart += len(pair) + 1
	}
	return spans, nil
}

type httpBody int

const (
	httpBodyText httpBody = iota
	httpBodyJSON
	httpBodyForm
)

// httpBodyMode decides how body placeholders are encoded, from the first
// Content-Type header only: there is no sniffing of the body.
func httpBodyMode(headers []HTTPHeaderItem) httpBody {
	for _, h := range headers {
		if !isContentTypeHeader(h.Name) {
			continue
		}
		media, _, _ := strings.Cut(h.Value, ";")
		media = strings.ToLower(asciiTrim(media))
		switch {
		case media == "application/json" || strings.HasSuffix(media, "+json"):
			return httpBodyJSON
		case media == "application/x-www-form-urlencoded":
			return httpBodyForm
		}
		return httpBodyText
	}
	return httpBodyText
}

func isContentTypeHeader(name string) bool {
	return strings.ToLower(asciiTrim(name)) == "content-type"
}

// maxJSONTemplateDepth bounds nesting so a hostile body cannot exhaust the stack.
const maxJSONTemplateDepth = 512

type jsonTemplateScanner struct {
	s     string
	i     int
	depth int
	spans []httpSpan
}

func invalidJSON(detail string) *HTTPTemplateError {
	return &HTTPTemplateError{Code: HTTPErrInvalidJSON, Position: HTTPPositionBody, Detail: "the JSON body is not valid: " + detail}
}

// jsonBodySpans walks a JSON body template: placeholders inside string
// literals are json-string uses, a bare placeholder in a value position is a
// json-value use, and a placeholder anywhere else is refused.
func jsonBodySpans(body string) ([]httpSpan, error) {
	p := &jsonTemplateScanner{s: body}
	if err := p.value(); err != nil {
		return nil, err
	}
	p.ws()
	if p.i < len(p.s) {
		return nil, p.unexpected()
	}
	return p.spans, nil
}

func (p *jsonTemplateScanner) ws() {
	for p.i < len(p.s) && strings.IndexByte(" \t\r\n", p.s[p.i]) >= 0 {
		p.i++
	}
}

// placeholderAt returns the placeholder starting at the cursor and its length.
func (p *jsonTemplateScanner) placeholderAt() (string, int) {
	if p.i < len(p.s) && p.s[p.i] == '{' {
		loc := httpPlaceholderPrefixRe.FindStringSubmatchIndex(p.s[p.i:])
		if loc != nil && !isLiteralBraces(p.s, p.i, p.i+loc[1]) {
			return p.s[p.i+loc[2] : p.i+loc[3]], loc[1]
		}
	}
	return "", 0
}

func (p *jsonTemplateScanner) unexpected() *HTTPTemplateError {
	if name, _ := p.placeholderAt(); name != "" {
		return forbidden(name, HTTPPositionJSONOther, "", "a placeholder in a JSON body must be a whole value or sit inside a string")
	}
	if p.i >= len(p.s) {
		return invalidJSON("unexpected end of input")
	}
	return invalidJSON(fmt.Sprintf("unexpected character at offset %d", p.i))
}

func (p *jsonTemplateScanner) value() *HTTPTemplateError {
	p.ws()
	if name, n := p.placeholderAt(); n > 0 {
		p.spans = append(p.spans, httpSpan{
			HTTPPlaceholder: HTTPPlaceholder{Name: name, Position: HTTPPositionJSONValue},
			start:           p.i, end: p.i + n,
		})
		p.i += n
		return nil
	}
	if p.i >= len(p.s) {
		return p.unexpected()
	}
	switch c := p.s[p.i]; {
	case c == '{' || c == '[':
		return p.container(c)
	case c == '"':
		return p.str(false)
	case c == '-' || (c >= '0' && c <= '9'):
		return p.number()
	}
	for _, lit := range []string{"true", "false", "null"} {
		if strings.HasPrefix(p.s[p.i:], lit) {
			p.i += len(lit)
			return nil
		}
	}
	return p.unexpected()
}

func (p *jsonTemplateScanner) container(open byte) *HTTPTemplateError {
	p.depth++
	if p.depth > maxJSONTemplateDepth {
		return invalidJSON("nesting is too deep")
	}
	closing := byte(']')
	if open == '{' {
		closing = '}'
	}
	p.i++
	p.ws()
	if p.i < len(p.s) && p.s[p.i] == closing {
		p.i++
		p.depth--
		return nil
	}
	for {
		if open == '{' {
			p.ws()
			if name, _ := p.placeholderAt(); name != "" {
				return forbidden(name, HTTPPositionJSONKey, "", "an object key cannot hold a placeholder")
			}
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return p.unexpected()
			}
			if err := p.str(true); err != nil {
				return err
			}
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return p.unexpected()
			}
			p.i++
		}
		if err := p.value(); err != nil {
			return err
		}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ',' {
			p.i++
			continue
		}
		if p.i < len(p.s) && p.s[p.i] == closing {
			p.i++
			p.depth--
			return nil
		}
		return p.unexpected()
	}
}

func (p *jsonTemplateScanner) str(isKey bool) *HTTPTemplateError {
	p.i++ // opening quote
	start := p.i
	for {
		if p.i >= len(p.s) {
			return invalidJSON("unterminated string")
		}
		c := p.s[p.i]
		switch {
		case c == '"':
			raw := p.s[start:p.i]
			for _, m := range placeholderMatches(raw) {
				name := raw[m[2]:m[3]]
				if isKey {
					return forbidden(name, HTTPPositionJSONKey, "", "an object key cannot hold a placeholder")
				}
				p.spans = append(p.spans, httpSpan{
					HTTPPlaceholder: HTTPPlaceholder{Name: name, Position: HTTPPositionJSONString},
					start:           start + m[0], end: start + m[1],
				})
			}
			p.i++
			return nil
		case c == '\\':
			p.i++
			if p.i >= len(p.s) {
				return invalidJSON("unterminated string")
			}
			switch e := p.s[p.i]; {
			case strings.IndexByte(`"\/bfnrt`, e) >= 0:
				p.i++
			case e == 'u' && p.i+4 < len(p.s) && isHex4(p.s[p.i+1:p.i+5]):
				p.i += 5
			default:
				return invalidJSON(fmt.Sprintf("invalid escape at offset %d", p.i))
			}
		case c < 0x20:
			return invalidJSON(fmt.Sprintf("control character in a string at offset %d", p.i))
		default:
			p.i++
		}
	}
}

func isHex4(s string) bool {
	for i := 0; i < 4; i++ {
		if strings.IndexByte("0123456789abcdefABCDEF", s[i]) < 0 {
			return false
		}
	}
	return true
}

var jsonNumberRe = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?`)

func (p *jsonTemplateScanner) number() *HTTPTemplateError {
	n := len(jsonNumberRe.FindString(p.s[p.i:]))
	if n == 0 {
		return invalidJSON(fmt.Sprintf("invalid number at offset %d", p.i))
	}
	p.i += n
	return nil
}

// ValidateHTTPParameterDefs checks the declarations of an HTTP query's
// parameters on their own (REQ:declare-parameters, REQ:credentials-run-time-only).
func ValidateHTTPParameterDefs(params Parameters) error {
	seen := make(map[string]bool, len(params))
	for _, p := range params {
		bad := func(detail string) error {
			return &HTTPTemplateError{Code: HTTPErrInvalidDeclaration, Parameter: p.ID, Detail: detail}
		}
		if !httpParamIDRe.MatchString(p.ID) {
			return bad("an HTTP parameter id uses only letters, digits and _")
		}
		if seen[p.ID] {
			return &HTTPTemplateError{Code: HTTPErrDuplicateParameter, Parameter: p.ID, Detail: "is declared more than once"}
		}
		seen[p.ID] = true
		if !isHTTPParameterType(p.Type) {
			return bad("type must be one of " + strings.Join(HTTPParameterTypes, ", "))
		}
		if p.IsMultiValue || p.Lookup != nil || p.MinLength != 0 || p.MaxLength != 0 {
			return bad("isMultiValue, lookup, minLength and maxLength are not supported on HTTP queries")
		}
		if p.Meta != nil {
			if err := p.Meta.Validate(); err != nil {
				return bad("meta: " + err.Error())
			}
		}
		if p.IsCredential {
			if !p.IsRequired || p.DefaultValue != nil || p.Meta != nil {
				return &HTTPTemplateError{Code: HTTPErrCredential, Parameter: p.ID,
					Detail: "a credential parameter must be required, with no default and no meta: its value is typed for each run"}
			}
			continue
		}
		if p.DefaultValue == "" {
			return bad("a default value cannot be empty")
		}
		if p.DefaultValue != nil {
			if err := p.Validate(); err != nil {
				return bad("defaultValue: " + err.Error())
			}
			if _, ok := RenderHTTPParameterValue(p.Type, p.DefaultValue); !ok {
				return bad("defaultValue does not match type " + p.Type)
			}
		} else if !p.IsRequired {
			return &HTTPTemplateError{Code: HTTPErrDefaultRequired, Parameter: p.ID,
				Detail: "a parameter that is not required needs a default value"}
		}
	}
	return nil
}

func isHTTPParameterType(t string) bool {
	for _, allowed := range HTTPParameterTypes {
		if t == allowed {
			return true
		}
	}
	return false
}

func isJSONLiteralType(t string) bool {
	switch t {
	case "integer", "number", "boolean", "bit":
		return true
	}
	return false
}

// checkUses matches placeholders against declarations.
func checkUses(spans []httpSpan, params Parameters) (map[string]ParameterDef, error) {
	defs := make(map[string]ParameterDef, len(params))
	for _, p := range params {
		defs[p.ID] = p
	}
	used := make(map[string]bool, len(spans))
	for _, s := range spans {
		def, ok := defs[s.Name]
		if !ok {
			return nil, &HTTPTemplateError{Code: HTTPErrUndeclared, Parameter: s.Name, Position: s.Position, Header: s.Header,
				Detail: "is used as a placeholder but not declared"}
		}
		used[s.Name] = true
		switch {
		case s.Credential && !def.IsCredential:
			return nil, &HTTPTemplateError{Code: HTTPErrCredential, Parameter: s.Name, Position: s.Position, Header: s.Header,
				Detail: "a parameter used in a credential header must be declared as a credential"}
		case !s.Credential && def.IsCredential:
			return nil, &HTTPTemplateError{Code: HTTPErrCredential, Parameter: s.Name, Position: s.Position, Header: s.Header,
				Detail: "a credential parameter can only be used in credential headers"}
		case s.Position == HTTPPositionJSONValue && !isJSONLiteralType(def.Type):
			return nil, &HTTPTemplateError{Code: HTTPErrJSONType, Parameter: s.Name, Position: s.Position,
				Detail: "only integer, number, boolean and bit parameters can be a bare JSON value; put a " + def.Type + " parameter inside quotes"}
		}
	}
	for _, p := range params {
		// Credential headers are never saved, so a saved credential
		// parameter legitimately has no placeholder in the saved template.
		if !used[p.ID] && !p.IsCredential {
			return nil, &HTTPTemplateError{Code: HTTPErrUnused, Parameter: p.ID, Detail: "is declared but no placeholder uses it"}
		}
	}
	return defs, nil
}

// ValidateParameters is the save-time check of a template with its declared
// parameters: declarations, positions, JSON structure and the rule that
// every placeholder is declared and every declaration is used.
func (t HTTPRequestTemplate) ValidateParameters(params Parameters) error {
	if err := ValidateHTTPParameterDefs(params); err != nil {
		return err
	}
	spans, err := t.spans()
	if err != nil {
		return err
	}
	_, err = checkUses(spans, params)
	return err
}

// Substitute validates the template like ValidateParameters and replaces each
// placeholder with its value, encoded for its position. values maps a
// parameter id to a string, number or boolean; a missing, nil or empty value
// takes the declared default, else fails with missing-value. Substituted text
// is never rescanned.
func (t HTTPRequestTemplate) Substitute(params Parameters, values map[string]any) (HTTPRequestTemplate, error) {
	if err := ValidateHTTPParameterDefs(params); err != nil {
		return t, err
	}
	spans, err := t.spans()
	if err != nil {
		return t, err
	}
	defs, err := checkUses(spans, params)
	if err != nil {
		return t, err
	}
	rendered := make(map[string]string, len(defs))
	for _, s := range spans {
		if _, done := rendered[s.Name]; done {
			continue
		}
		def := defs[s.Name]
		v := values[s.Name]
		if isMissingHTTPValue(v) {
			v = def.DefaultValue
		}
		if isMissingHTTPValue(v) {
			return t, &HTTPTemplateError{Code: HTTPErrMissingValue, Parameter: s.Name, Detail: "has no value"}
		}
		text, ok := RenderHTTPParameterValue(def.Type, v)
		if !ok {
			return t, &HTTPTemplateError{Code: HTTPErrInvalidValue, Parameter: s.Name, Detail: "the value is not a valid " + def.Type}
		}
		rendered[s.Name] = text
	}
	// Encode in template order so the first offending placeholder is named.
	// A path value of exactly . or .. is the only way a segment can become a
	// dot segment, since PathEscape keeps '.' and escapes '/'.
	// Each (parameter, position) pair is encoded once, so a large value used
	// by many placeholders costs one copy until spliceField caps the output.
	type encodingKey struct {
		name string
		pos  HTTPPosition
	}
	cache := make(map[encodingKey]string, len(rendered))
	encoded := make([]string, len(spans))
	for i, s := range spans {
		key := encodingKey{s.Name, s.Position}
		if e, ok := cache[key]; ok && s.Position != HTTPPositionHeader {
			encoded[i] = e
			continue
		}
		value := rendered[s.Name]
		switch s.Position {
		case HTTPPositionPath:
			if value == "." || value == ".." {
				return t, &HTTPTemplateError{Code: HTTPErrDotSegment, Parameter: s.Name, Position: s.Position,
					Detail: "a path value cannot be . or .."}
			}
			encoded[i] = url.PathEscape(value)
		case HTTPPositionQuery:
			encoded[i] = url.QueryEscape(value)
		case HTTPPositionHeader:
			if strings.IndexFunc(value, isHeaderControl) >= 0 {
				return t, &HTTPTemplateError{Code: HTTPErrHeaderLineBreak, Parameter: s.Name, Position: s.Position, Header: s.Header,
					Detail: "a header value cannot contain a line break, NUL or other control character"}
			}
			encoded[i] = value
		case HTTPPositionJSONString:
			encoded[i] = jsonStringContent(value)
		case HTTPPositionBody:
			if httpBodyMode(t.Headers) == httpBodyForm {
				encoded[i] = url.QueryEscape(value)
			} else {
				encoded[i] = value
			}
		default: // HTTPPositionJSONValue: a rendered integer, number, boolean or bit is a JSON literal.
			encoded[i] = value
		}
		cache[key] = encoded[i]
	}
	out := HTTPRequestTemplate{Method: t.Method}
	if out.URL, err = spliceField(asciiTrim(t.URL), spans, encoded, httpFieldURL, 0, MaxHTTPTemplateURLBytes, HTTPPositionPath); err != nil {
		return t, err
	}
	if out.Body, err = spliceField(t.Body, spans, encoded, httpFieldBody, 0, MaxQueryHTTPBodyBytes, HTTPPositionBody); err != nil {
		return t, err
	}
	if t.Headers != nil {
		out.Headers = make([]HTTPHeaderItem, len(t.Headers))
		for i, h := range t.Headers {
			out.Headers[i] = h
			if out.Headers[i].Value, err = spliceField(h.Value, spans, encoded, httpFieldHeader, i, MaxHTTPTemplateHeaderBytes, HTTPPositionHeader); err != nil {
				return t, err
			}
		}
	}
	return out, nil
}

// isHeaderControl reports a character a header value cannot carry: any
// ASCII control character except horizontal tab (as Go's net/http).
func isHeaderControl(r rune) bool {
	return (r < 0x20 && r != '\t') || r == 0x7f
}

// spliceField rebuilds one field in a single pass (a field with no spans is
// returned as is: its size is bounded by the caller's own request limits), replacing its spans (in
// template order) with their encoded values, and fails once the result would
// exceed max bytes.
func spliceField(text string, spans []httpSpan, encoded []string, field httpField, header, max int, pos HTTPPosition) (string, error) {
	var b strings.Builder
	last := 0
	for i, s := range spans {
		if s.field != field || s.header != header {
			continue
		}
		if b.Len()+(s.start-last)+len(encoded[i]) > max {
			return "", &HTTPTemplateError{Code: HTTPErrTooLarge, Parameter: s.Name, Position: pos,
				Detail: fmt.Sprintf("the substituted value makes it larger than %d bytes", max)}
		}
		b.WriteString(text[last:s.start])
		b.WriteString(encoded[i])
		last = s.end
	}
	if last == 0 {
		return text, nil
	}
	if b.Len()+len(text)-last > max {
		return "", &HTTPTemplateError{Code: HTTPErrTooLarge, Position: pos,
			Detail: fmt.Sprintf("the substituted value makes it larger than %d bytes", max)}
	}
	b.WriteString(text[last:])
	return b.String(), nil
}

func isMissingHTTPValue(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && s == ""
}

// jsonStringContent escapes s for the inside of a JSON string literal: ",
// \ and control characters only, with no added quotes.
func jsonStringContent(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// SubstituteHTTPRunParameters is the server's template mode
// (REQ:server-template-endpoint): the run carries only {id, type, value} for
// each parameter, so every supplied parameter is required, a parameter used
// in a credential header is a credential, and the full template rules apply.
func SubstituteHTTPRunParameters(t HTTPRequestTemplate, params []Parameter) (HTTPRequestTemplate, error) {
	spans, err := t.spans()
	if err != nil {
		return t, err
	}
	credential := map[string]bool{}
	for _, s := range spans {
		if s.Credential {
			credential[s.Name] = true
		}
	}
	defs := make(Parameters, 0, len(params))
	values := make(map[string]any, len(params))
	for _, p := range params {
		defs = append(defs, ParameterDef{ID: p.ID, Type: p.Type, IsRequired: true, IsCredential: credential[p.ID]})
		values[p.ID] = p.Value
	}
	return t.Substitute(defs, values)
}

// RenderHTTPParameterValue coerces a bound value to the parameter type and
// renders it as text (REQ:parameter-meta, REQ:type-coercion): string as is,
// integer in decimal, number in shortest decimal form without an exponent,
// boolean as true/false, bit as 0/1, date as YYYY-MM-DD and datetime as
// RFC 3339 UTC. ok is false when the value cannot be coerced.
func RenderHTTPParameterValue(paramType string, v any) (string, bool) {
	switch paramType {
	case "string":
		switch x := v.(type) {
		case string:
			return x, true
		case bool:
			if x {
				return "true", true
			}
			return "false", true
		}
		if f, ok := httpNumber(v); ok {
			return formatHTTPNumber(f), true
		}
	case "integer":
		if s, ok := v.(string); ok {
			return parseHTTPInteger(s)
		}
		if f, ok := httpNumber(v); ok && f == float64(int64(f)) && f >= -maxSafeHTTPInteger && f <= maxSafeHTTPInteger {
			return formatHTTPNumber(f), true
		}
	case "number":
		if s, ok := v.(string); ok {
			if !jsonNumberFullRe.MatchString(s) {
				return "", false
			}
			v = json.Number(s)
		}
		if f, ok := httpNumber(v); ok {
			return formatHTTPNumber(f), true
		}
	case "boolean", "bit":
		b, ok := httpBool(v)
		if !ok {
			return "", false
		}
		switch {
		case paramType == "bit" && b:
			return "1", true
		case paramType == "bit":
			return "0", true
		case b:
			return "true", true
		}
		return "false", true
	case "date", "datetime":
		s, ok := v.(string)
		if !ok {
			return "", false
		}
		return renderHTTPTime(paramType, s)
	}
	return "", false
}

// HTTPTemplate is the request template of an HTTP query: its URL (Text) and
// saved method, headers and body.
func (v QueryDef) HTTPTemplate() HTTPRequestTemplate {
	t := HTTPRequestTemplate{URL: v.Text}
	if v.HTTP != nil {
		t.Method, t.Headers, t.Body = v.HTTP.Method, v.HTTP.Headers, v.HTTP.Body
	}
	return t
}

// validateHTTPParameters applies the placeholder contract to an HTTP query
// that declares parameters. A query with no declared parameters is a legacy
// literal request and keeps running as stored (REQ:legacy-literal-braces).
func (v QueryDef) validateHTTPParameters() error {
	if v.Type != QueryTypeHTTP {
		for _, p := range v.Parameters {
			if p.IsCredential {
				return &HTTPTemplateError{Code: HTTPErrInvalidDeclaration, Parameter: p.ID, Detail: "only HTTP query parameters can be credentials"}
			}
		}
		return nil
	}
	if len(v.Parameters) == 0 {
		return nil
	}
	return v.HTTPTemplate().ValidateParameters(v.Parameters)
}

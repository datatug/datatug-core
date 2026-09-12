package datatug

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/strongo/validation"
)

// Origins a captured parameter binding may record. A captured query stores
// how each parameter was bound while the user explored, not the bound
// value: default values are never captured (a captured value would put a
// possibly protected record identifier into git-tracked project files), so
// there is no "default" origin here. Every origin is client-reported; the
// server never attests it.
const (
	QueryCaptureOriginSelection = "selection"
	QueryCaptureOriginContext   = "context"
	QueryCaptureOriginManual    = "manual"
)

// QueryCapture records where a query captured from exploration came from,
// persisted inside the query's own "<id>.query.json" so the provenance
// travels with the query through git. Hub datatug/datatug
// executable-knowledge-library REQ:capture-from-exploration: "Query author,
// purpose and source/binding provenance are inspectable."
//
// It carries identifiers only - never a bound value, a result row or a
// credential.
type QueryCapture struct {
	// Author is the serving principal the capture was saved under, set by
	// the server, never by the client. Empty when the server ran with no
	// identified principal.
	Author string `json:"author,omitempty" yaml:"author,omitempty"`

	// Environment is the environment the captured lookup ran in.
	Environment string `json:"environment" yaml:"environment"`

	// Source is the stable project-local source ID (api-contract.md
	// SourceRef.source) the captured lookup read.
	Source string `json:"source" yaml:"source"`

	// Collection is the collection the captured query reads, derived by the
	// server from the query itself.
	Collection string `json:"collection,omitempty" yaml:"collection,omitempty"`

	// Bindings records how each parameter was bound during capture.
	Bindings []QueryCaptureBinding `json:"bindings,omitempty" yaml:"bindings,omitempty"`
}

// QueryCaptureBinding is one parameter's capture-time binding origin.
type QueryCaptureBinding struct {
	ParameterID string `json:"parameterId" yaml:"parameterId"`
	Origin      string `json:"origin" yaml:"origin"` // selection | context | manual
}

// Capture provenance is screened by shape, not by
// EmbeddedCredentialReason (query_credentials.go). Every string a capture
// records - the author, the environment, the source, the collection and
// each binding's parameter ID - is an identifier or a value the server
// recorded, never free text, so a rule about the value's shape says
// everything that needs saying about it and, unlike the credential
// screen, never has to guess what a word means.
//
// That matters because the credential screen is tuned for free text and
// has documented false positives a realistic capture would hit: it reads
// "from:alice@example.com" as DSN userinfo, and it refuses any key/value
// pair whose key ends in a secret suffix whatever the value holds. An
// author is usually an e-mail address and a collection may well be named
// "passwords" or "password_resets", so screening these fields that way
// would refuse valid provenance.
//
// The shape rule is not the weaker of the two. Each of the five syntaxes
// EmbeddedCredentialReason recognizes needs punctuation this rule
// forbids: a URL userinfo needs "://", a DSN userinfo needs ":", a
// key/value pair needs "=", a JSON member needs a double quote and ":",
// and an HTTP credential header needs ":". A value holding none of ':',
// '=' and '"' therefore cannot express any of them and cannot trip the
// credential screen either - which is what
// TestCaptureShape_IsNotWeakerThanTheCredentialScreen proves, in both
// directions, on the screen's own probes. Control characters and line
// breaks are refused as well, so no field can smuggle a second line (an
// "Authorization:" header line, say) into the file; so is a value that is
// not valid UTF-8, which would reach the JSON as replacement characters,
// and one padded with spaces, which names nothing that a lookup would
// find.
const maxCaptureFieldLength = 200

// Refusal reasons returned by captureShapeReason.
const (
	captureShapeWhitespaceReason  = "must not start or end with whitespace"
	captureShapeEncodingReason    = "must be valid UTF-8"
	captureShapeControlReason     = "must not contain control characters or line breaks"
	captureShapePunctuationReason = `must not contain ':', '=' or '"': a capture records an identifier, not a connection string, a key/value pair or a JSON object`
)

// captureShapeReason reports why value cannot be stored as capture
// provenance (see the shape notes above), or ("", false) when it can. An
// empty value passes: which provenance fields are required is decided by
// QueryCapture.Validate, not here.
func captureShapeReason(value string) (reason string, found bool) {
	if value == "" {
		return "", false
	}
	if strings.TrimSpace(value) != value {
		return captureShapeWhitespaceReason, true
	}
	if !utf8.ValidString(value) {
		return captureShapeEncodingReason, true
	}
	if len(value) > maxCaptureFieldLength {
		return fmt.Sprintf("exceeds max length (%d): %d", maxCaptureFieldLength, len(value)), true
	}
	for _, r := range value {
		switch {
		case r == ':' || r == '=' || r == '"':
			return captureShapePunctuationReason, true
		case unicode.IsControl(r):
			return captureShapeControlReason, true
		}
	}
	return "", false
}

// Validate returns an error if the capture provenance is incomplete: an
// environment and a source are required, and every binding must name a
// parameter, at most once, with one of the capture origins. A capture is
// persisted inside the git-tracked "<id>.query.json", so every string it
// records is screened for credential material too - by shape, for the
// reasons given above.
func (v QueryCapture) Validate() error {
	if strings.TrimSpace(v.Environment) == "" {
		return validation.NewErrRecordIsMissingRequiredField("environment")
	}
	if strings.TrimSpace(v.Source) == "" {
		return validation.NewErrRecordIsMissingRequiredField("source")
	}
	for _, f := range []struct{ name, value string }{
		{"author", v.Author}, {"environment", v.Environment}, {"source", v.Source}, {"collection", v.Collection},
	} {
		if reason, found := captureShapeReason(f.value); found {
			return validation.NewErrBadRecordFieldValue(f.name, reason)
		}
	}
	seen := make(map[string]bool, len(v.Bindings))
	for i, b := range v.Bindings {
		field := fmt.Sprintf("bindings[%d]", i)
		if strings.TrimSpace(b.ParameterID) == "" {
			return validation.NewErrBadRecordFieldValue(field, "parameterId is required")
		}
		if reason, found := captureShapeReason(b.ParameterID); found {
			return validation.NewErrBadRecordFieldValue(field, "parameterId "+reason)
		}
		switch b.Origin {
		case QueryCaptureOriginSelection, QueryCaptureOriginContext, QueryCaptureOriginManual:
		default:
			return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("origin must be one of %s, %s or %s, got %q",
				QueryCaptureOriginSelection, QueryCaptureOriginContext, QueryCaptureOriginManual, b.Origin))
		}
		if seen[b.ParameterID] {
			return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("duplicate binding for parameter %q", b.ParameterID))
		}
		seen[b.ParameterID] = true
	}
	return nil
}

// validateCaptureAgainst checks v on its own and then that every binding
// names one of parameters.
func (v QueryCapture) validateCaptureAgainst(parameters Parameters) error {
	if err := v.Validate(); err != nil {
		return err
	}
	declared := make(map[string]bool, len(parameters))
	for _, p := range parameters {
		declared[p.ID] = true
	}
	for i, b := range v.Bindings {
		if !declared[b.ParameterID] {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("bindings[%d]", i),
				fmt.Sprintf("parameter %q is not declared by the query", b.ParameterID))
		}
	}
	return nil
}

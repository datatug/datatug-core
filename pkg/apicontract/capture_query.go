package apicontract

import (
	"errors"
	"fmt"
	"strings"
)

// Capture as project query: POST queries/capture (hub datatug/datatug
// executable-knowledge-library REQ:capture-from-exploration, Phase 2 plan
// task 2). The browser saves a lookup it already ran as an ordinary project
// query - a "<id>.query.json" / "<id>.query.dtql" pair left as an
// uncommitted Git change - with its purpose, typed parameters (semantic
// ones declaring Meta as an EntityFieldRef) and source/binding provenance.
//
// The request carries no result rows, no bound or default values, no
// session-local fact IDs and no credentials; DecodeStrict rejects any field
// not declared here. It carries exactly one optimistic-concurrency
// condition: IfNoneMatch (create only when nothing exists at the query's
// location) or IfMatch (replace only the revision the caller last read).
//
// Lead assumption 2026-09-11, pending an api-contract.md amendment: this
// endpoint, its shapes and ErrCodeRevisionConflict are not yet in the
// appendix's endpoint table or closed error-code set.

// CaptureQueryRequest is POST queries/capture's request body. Like every
// other POST envelope here it flattens Scope into its own top level.
type CaptureQueryRequest struct {
	Project           string        `json:"project"`
	Environment       string        `json:"environment"`
	SecurityContextID string        `json:"securityContextId"`
	IfNoneMatch       bool          `json:"ifNoneMatch,omitempty"`
	IfMatch           string        `json:"ifMatch,omitempty"`
	Query             CapturedQuery `json:"query"`
}

// CapturedQuery is the persisted definition of a captured query, as the
// client submits it and as the server returns it once stored.
type CapturedQuery struct {
	// FolderPath is the "/"-separated folder under the project's queries/
	// root; "" is the root itself.
	FolderPath string `json:"folderPath"`
	// ID is the query's bare ID: one file-name segment.
	ID      string `json:"id"`
	Title   string `json:"title"`
	Purpose string `json:"purpose"`
	// Source is the stable project-local source ID (SourceRef.source) the
	// captured lookup read; the server resolves it through the project
	// registry and binds the saved query to it.
	Source string `json:"source"`
	// DTQL is the query text persisted as "<id>.query.dtql".
	DTQL           string                 `json:"dtql"`
	Parameters     []CapturedParameter    `json:"parameters"`
	BindingOrigins []CaptureBindingOrigin `json:"bindingOrigins"`
}

// CapturedParameter is one typed parameter of a captured query. It has no
// default value: a captured default would persist a possibly protected
// value into git-tracked project files.
type CapturedParameter struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"` // string | number | integer | decimal | boolean | date | datetime
	Title      string          `json:"title,omitempty"`
	IsRequired bool            `json:"isRequired"`
	Meta       *EntityFieldRef `json:"meta,omitempty"`
}

// EntityFieldRef is a semantic parameter's declaration that it requires a
// value of one entity field ("requires Customer.ID") - datatug-core's
// ParameterDef.Meta on the wire.
type EntityFieldRef struct {
	Entity string `json:"entity"`
	Field  string `json:"field"`
}

// CaptureBindingOrigin records how one parameter was bound while the user
// explored. It is client-reported provenance, never server-attested, and
// carries no value and no fact ID.
type CaptureBindingOrigin struct {
	ParameterID string `json:"parameterId"`
	Origin      string `json:"origin"` // selection | context | manual
}

// CaptureQueryResponse is POST queries/capture's success envelope, sent
// only after the store has persisted the pair.
type CaptureQueryResponse struct {
	// QueryID is the stored query's canonical, folder-qualified ID
	// (CanonicalQueryID) - the queryId exec/run_query and
	// queries/applicable use.
	QueryID string `json:"queryId"`
	// Revision identifies the stored bytes; send it back as IfMatch to
	// replace this exact revision. Opaque: compare for equality only.
	Revision   string            `json:"revision"`
	Query      CapturedQuery     `json:"query"`
	Provenance CaptureProvenance `json:"provenance"`
}

// CaptureProvenance is what the server itself recorded about a capture.
type CaptureProvenance struct {
	// Author is the serving principal; omitted when the server runs with no
	// identified principal.
	Author string `json:"author,omitempty"`
	// Environment is the environment the source was resolved in.
	Environment string `json:"environment"`
	// Collection is the collection the query reads, derived from the DTQL.
	Collection string `json:"collection"`
}

// capturedParameterTypes is the closed set of parameter types a capture may
// declare: every TypedValue type except null.
var capturedParameterTypes = []string{
	string(ValueTypeString), string(ValueTypeNumber), string(ValueTypeInteger), string(ValueTypeDecimal),
	string(ValueTypeBoolean), string(ValueTypeDate), string(ValueTypeDatetime),
}

// CanonicalQueryID returns the folder-qualified query ID for a query with
// bare id under folderPath ("" for the queries root).
func CanonicalQueryID(folderPath, id string) string {
	if folderPath == "" {
		return id
	}
	return folderPath + "/" + id
}

// Validate enforces Scope's required fields, exactly one of IfNoneMatch or
// IfMatch, and the query's own rules (CapturedQuery.Validate), reporting a
// query field as "query.<field>".
func (r CaptureQueryRequest) Validate() error {
	if err := (Scope{Project: r.Project, Environment: r.Environment, SecurityContextID: r.SecurityContextID}).Validate(); err != nil {
		return err
	}
	if r.IfNoneMatch == (r.IfMatch != "") {
		return &ValidationError{Field: "ifNoneMatch/ifMatch", Message: "exactly one of ifNoneMatch or ifMatch is required"}
	}
	return prefixField("query", r.Query.Validate())
}

// Validate enforces the shape rules both server and client apply: ID is one
// path segment and FolderPath is "" or "/"-separated segments, none empty,
// "." or ".." and none holding a backslash or NUL (the server applies its
// stricter file-name rules on top); Title, Purpose, Source and DTQL are
// required; every parameter has a unique ID, a type from the closed set and,
// when present, a complete Meta; every binding origin names a declared
// parameter at most once with origin selection, context or manual.
func (q CapturedQuery) Validate() error {
	if err := requireNonEmpty("id", q.ID); err != nil {
		return err
	}
	if reason, ok := querySegmentReason(q.ID); !ok || strings.Contains(q.ID, "/") {
		if ok {
			reason = "must be one path segment, without \"/\""
		}
		return &ValidationError{Field: "id", Message: reason}
	}
	if q.FolderPath != "" {
		for _, segment := range strings.Split(q.FolderPath, "/") {
			if reason, ok := querySegmentReason(segment); !ok {
				return &ValidationError{Field: "folderPath", Message: fmt.Sprintf("segment %q %s", segment, reason)}
			}
		}
	}
	for _, f := range []struct{ name, value string }{
		{"title", q.Title}, {"purpose", q.Purpose}, {"source", q.Source}, {"dtql", q.DTQL},
	} {
		if err := requireNonEmpty(f.name, f.value); err != nil {
			return err
		}
	}
	declared := make(map[string]bool, len(q.Parameters))
	for i, p := range q.Parameters {
		if err := p.Validate(); err != nil {
			return &ValidationError{Field: "parameters", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if declared[p.ID] {
			return &ValidationError{Field: "parameters", Message: fmt.Sprintf("index %d: duplicate parameter %q", i, p.ID)}
		}
		declared[p.ID] = true
	}
	bound := make(map[string]bool, len(q.BindingOrigins))
	for i, b := range q.BindingOrigins {
		if err := b.Validate(); err != nil {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if !declared[b.ParameterID] {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("index %d: names parameter %q, which is not declared", i, b.ParameterID)}
		}
		if bound[b.ParameterID] {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("index %d: duplicate entry for parameter %q", i, b.ParameterID)}
		}
		bound[b.ParameterID] = true
	}
	return nil
}

// Validate enforces a required ID, a type from the closed set and, when
// Meta is present, both its entity and field.
func (p CapturedParameter) Validate() error {
	if err := requireNonEmpty("id", p.ID); err != nil {
		return err
	}
	if err := requireOneOf("type", p.Type, capturedParameterTypes...); err != nil {
		return err
	}
	if p.Meta != nil {
		if err := requireNonEmpty("meta.entity", p.Meta.Entity); err != nil {
			return err
		}
		if err := requireNonEmpty("meta.field", p.Meta.Field); err != nil {
			return err
		}
	}
	return nil
}

// Validate enforces a required ParameterID and an origin of selection,
// context or manual - a default value is never captured, so "default" is
// not a capture origin.
func (b CaptureBindingOrigin) Validate() error {
	if err := requireNonEmpty("parameterId", b.ParameterID); err != nil {
		return err
	}
	return requireOneOf("origin", b.Origin, BindingOriginSelection, BindingOriginContext, BindingOriginManual)
}

// Validate enforces QueryID and Revision are present, QueryID is the
// query's own canonical ID, the query is itself valid and the provenance
// names its environment and collection.
func (r CaptureQueryResponse) Validate() error {
	if err := requireNonEmpty("queryId", r.QueryID); err != nil {
		return err
	}
	if err := requireNonEmpty("revision", r.Revision); err != nil {
		return err
	}
	if err := prefixField("query", r.Query.Validate()); err != nil {
		return err
	}
	if want := CanonicalQueryID(r.Query.FolderPath, r.Query.ID); r.QueryID != want {
		return &ValidationError{Field: "queryId", Message: fmt.Sprintf("must be the query's canonical ID %q, got %q", want, r.QueryID)}
	}
	if err := requireNonEmpty("environment", r.Provenance.Environment); err != nil {
		return prefixField("provenance", err)
	}
	if err := requireNonEmpty("collection", r.Provenance.Collection); err != nil {
		return prefixField("provenance", err)
	}
	return nil
}

// querySegmentReason reports why segment cannot name one folder or query
// file under the queries root, or ("", true) when it can.
func querySegmentReason(segment string) (reason string, ok bool) {
	switch {
	case segment == "":
		return "must not be empty", false
	case segment == "." || segment == "..":
		return `must not be "." or ".."`, false
	case strings.ContainsAny(segment, "\\\x00"):
		return "must not contain a backslash or NUL", false
	}
	return "", true
}

// prefixField re-reports a *ValidationError under parent ("query.id"),
// passing nil and any other error through unchanged.
func prefixField(parent string, err error) error {
	var ve *ValidationError
	if !errors.As(err, &ve) {
		return err
	}
	return &ValidationError{Field: parent + "." + ve.Field, Message: ve.Message}
}

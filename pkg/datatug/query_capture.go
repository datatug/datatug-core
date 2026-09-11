package datatug

import (
	"fmt"
	"strings"

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

// Validate returns an error if the capture provenance is incomplete: an
// environment and a source are required, and every binding must name a
// parameter, at most once, with one of the capture origins.
func (v QueryCapture) Validate() error {
	if strings.TrimSpace(v.Environment) == "" {
		return validation.NewErrRecordIsMissingRequiredField("environment")
	}
	if strings.TrimSpace(v.Source) == "" {
		return validation.NewErrRecordIsMissingRequiredField("source")
	}
	seen := make(map[string]bool, len(v.Bindings))
	for i, b := range v.Bindings {
		field := fmt.Sprintf("bindings[%d]", i)
		if strings.TrimSpace(b.ParameterID) == "" {
			return validation.NewErrBadRecordFieldValue(field, "parameterId is required")
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

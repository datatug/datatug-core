package apicontract

import "fmt"

// BindingOriginEntry supplies display provenance for one submitted
// parameter - "never influences authorization". api-contract.md
// "Endpoint table".
type BindingOriginEntry struct {
	ParameterID string `json:"parameterId"`
	Origin      string `json:"origin"` // selection | context | manual | default
	FactID      string `json:"factId,omitempty"`
}

func (e BindingOriginEntry) Validate() error {
	if err := requireNonEmpty("parameterId", e.ParameterID); err != nil {
		return err
	}
	return requireOneOf("origin", e.Origin, BindingOriginSelection, BindingOriginContext, BindingOriginManual, BindingOriginDefault)
}

// ExecutionRequest is POST exec/run_query's request body. "For ad-hoc DTQL,
// source is required. For saved queries, source obeys target resolution...
// Unknown parameters and type mismatches are rejected. The server never
// silently binds from stored browser context." api-contract.md
// "Endpoint table".
type ExecutionRequest struct {
	StoreID           string                `json:"storeId,omitempty"`
	Project           string                `json:"project"`
	Environment       string                `json:"environment"`
	SecurityContextID string                `json:"securityContextId"`
	Source            string                `json:"source,omitempty"`
	QueryID           string                `json:"queryId,omitempty"`
	DTQL              string                `json:"dtql,omitempty"`
	Parameters        map[string]TypedValue `json:"parameters"`
	BindingOrigins    []BindingOriginEntry  `json:"bindingOrigins"`
	Mode              string                `json:"mode"` // live | snapshot
	SnapshotID        string                `json:"snapshotId,omitempty"`
	Limit             *int                  `json:"limit,omitempty"`
	Incident          *IncidentRef          `json:"incident,omitempty"`
}

const (
	executionMaxLimit = 500
)

// Validate enforces: Project/Environment/SecurityContextID required; exactly
// one of QueryID/DTQL ("exactly one required"); DTQL set requires Source
// ("For ad-hoc DTQL, source is required"); every Parameters value is itself
// valid; BindingOrigins is checked for exactly the submitted Parameters keys
// - no more, no fewer - and each entry is itself valid; Mode is one of the
// closed set, snapshot Mode requires a SnapshotID; Limit, when present, is
// within (0, 500] - "Default result limit is 100 and maximum is 500."
func (r ExecutionRequest) Validate() error {
	if r.StoreID != "" {
		if err := (Scope{StoreID: r.StoreID, Project: r.Project, Environment: r.Environment, SecurityContextID: r.SecurityContextID}).Validate(); err != nil {
			return err
		}
	}
	if err := requireNonEmpty("project", r.Project); err != nil {
		return err
	}
	if err := requireNonEmpty("environment", r.Environment); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", r.SecurityContextID); err != nil {
		return err
	}
	hasQueryID := r.QueryID != ""
	hasDTQL := r.DTQL != ""
	if hasQueryID == hasDTQL {
		return &ValidationError{Field: "queryId/dtql", Message: "exactly one of queryId or dtql is required"}
	}
	if hasDTQL && r.Source == "" {
		return &ValidationError{Field: "source", Message: "is required for ad-hoc dtql"}
	}
	for key, value := range r.Parameters {
		if err := value.Validate(); err != nil {
			return &ValidationError{Field: "parameters", Message: fmt.Sprintf("%s: %s", key, err)}
		}
	}
	submitted := make(map[string]bool, len(r.BindingOrigins))
	for i, e := range r.BindingOrigins {
		if err := e.Validate(); err != nil {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if submitted[e.ParameterID] {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("duplicate entry for parameter %q", e.ParameterID)}
		}
		submitted[e.ParameterID] = true
		if _, ok := r.Parameters[e.ParameterID]; !ok {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("names parameter %q, which is not in parameters", e.ParameterID)}
		}
	}
	for key := range r.Parameters {
		if !submitted[key] {
			return &ValidationError{Field: "bindingOrigins", Message: fmt.Sprintf("missing an entry for submitted parameter %q", key)}
		}
	}
	if err := requireOneOf("mode", r.Mode, ProvenanceModeLive, ProvenanceModeSnapshot); err != nil {
		return err
	}
	if r.Mode == ProvenanceModeSnapshot {
		if err := requireNonEmpty("snapshotId", r.SnapshotID); err != nil {
			return err
		}
	}
	if r.Limit != nil {
		if *r.Limit <= 0 || *r.Limit > executionMaxLimit {
			return &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d, got %d", executionMaxLimit, *r.Limit)}
		}
	}
	if r.Incident != nil {
		if err := r.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
		}
	}
	return nil
}

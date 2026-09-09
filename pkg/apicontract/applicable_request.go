package apicontract

import "fmt"

// ApplicableRequest is POST queries/applicable's request body: "Scope +
// {values:Fact[]}" - api-contract.md "Endpoint table". Scope's three fields
// are flattened into the top level of the JSON body, exactly like
// ExecutionRequest, never nested under a "scope" key - confirmed against the
// live server (Task 12 lane S77).
type ApplicableRequest struct {
	Project           string `json:"project"`
	Environment       string `json:"environment"`
	SecurityContextID string `json:"securityContextId"`
	Values            []Fact `json:"values"`
}

// Validate enforces Project/Environment/SecurityContextID are required (the
// same Scope invariants every scoped call carries - api-contract.md "Scope
// and identity") and every Values entry is itself valid. An empty Values
// list is legitimate: a caller with no known facts still gets back every
// query's notYet candidate.
func (r ApplicableRequest) Validate() error {
	if err := requireNonEmpty("project", r.Project); err != nil {
		return err
	}
	if err := requireNonEmpty("environment", r.Environment); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", r.SecurityContextID); err != nil {
		return err
	}
	for i, f := range r.Values {
		if err := f.Validate(); err != nil {
			return &ValidationError{Field: "values", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

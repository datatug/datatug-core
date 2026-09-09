package apicontract

import "fmt"

// RelatedRowsRequest is POST semantic/related/rows's request body: "Scope +
// {lookupId:string,value:TypedValue,limit?:number}" - api-contract.md
// "Endpoint table". Scope's three fields are flattened into the top level of
// the JSON body, exactly like ExecutionRequest, never nested under a
// "scope" key - confirmed against the live server (Task 12 lane S77).
// LookupID is "an opaque handle to a server-validated relationship... not
// authority. Each rows request revalidates the relationship, value type,
// current source policy and limit" (api-contract.md "Bounded lookups and
// HTTP") - this type only enforces it is present and nonempty; the server
// revalidates everything it names.
type RelatedRowsRequest struct {
	Project           string     `json:"project"`
	Environment       string     `json:"environment"`
	SecurityContextID string     `json:"securityContextId"`
	LookupID          string     `json:"lookupId"`
	Value             TypedValue `json:"value"`
	Limit             *int       `json:"limit,omitempty"`
}

// Validate enforces Project/Environment/SecurityContextID and LookupID are
// required, Value is itself valid, and Limit, when present, is within
// (0, executionMaxLimit] - this endpoint returns a Result, the same shape
// exec/run_query returns, so it is bound by the same "Default result limit
// is 100 and maximum is 500" rule (api-contract.md "Bounded lookups and
// HTTP") ExecutionRequest.Limit already enforces.
func (r RelatedRowsRequest) Validate() error {
	if err := requireNonEmpty("project", r.Project); err != nil {
		return err
	}
	if err := requireNonEmpty("environment", r.Environment); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", r.SecurityContextID); err != nil {
		return err
	}
	if err := requireNonEmpty("lookupId", r.LookupID); err != nil {
		return err
	}
	if err := r.Value.Validate(); err != nil {
		return &ValidationError{Field: "value", Message: err.Error()}
	}
	if r.Limit != nil {
		if *r.Limit <= 0 || *r.Limit > executionMaxLimit {
			return &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d, got %d", executionMaxLimit, *r.Limit)}
		}
	}
	return nil
}

package apicontract

import "fmt"

// RelatedRequest is POST semantic/related's request body: "Scope +
// {fact:Fact,limit?:number}" - api-contract.md "Endpoint table". Scope's
// three fields are flattened into the top level of the JSON body, exactly
// like ExecutionRequest, never nested under a "scope" key - confirmed
// against the live server (Task 12 lane S77). Related operations use POST so
// a semantic value is never copied into a URL, browser history or access
// log.
type RelatedRequest struct {
	Project           string `json:"project"`
	Environment       string `json:"environment"`
	SecurityContextID string `json:"securityContextId"`
	Fact              Fact   `json:"fact"`
	Limit             *int   `json:"limit,omitempty"`
}

// Validate enforces Project/Environment/SecurityContextID are required, Fact
// is itself valid, and Limit, when present, is within (0, relatedMaxItems] -
// "Related discovery returns at most 50 targets" (api-contract.md "Bounded
// lookups and HTTP") bounds what a caller may ask for, not just what a
// response may contain, so this reuses the same relatedMaxItems the
// RelatedResponse envelope is capped at (responses.go).
func (r RelatedRequest) Validate() error {
	if err := requireNonEmpty("project", r.Project); err != nil {
		return err
	}
	if err := requireNonEmpty("environment", r.Environment); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", r.SecurityContextID); err != nil {
		return err
	}
	if err := r.Fact.Validate(); err != nil {
		return &ValidationError{Field: "fact", Message: err.Error()}
	}
	if r.Limit != nil {
		if *r.Limit <= 0 || *r.Limit > relatedMaxItems {
			return &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d, got %d", relatedMaxItems, *r.Limit)}
		}
	}
	return nil
}

package apicontract

// Scope identifies the project, environment and staleness-check security
// context every scoped call carries. "Scope = {project: string, environment:
// string, securityContextId: string}" - api-contract.md "Scope and identity".
// Project and environment IDs are required, nonempty, and resolved within
// this server's registered projects; securityContextId is a staleness check,
// never authentication.
type Scope struct {
	Project           string `json:"project"`
	Environment       string `json:"environment"`
	SecurityContextID string `json:"securityContextId"`
}

// Validate enforces "Project and environment IDs are required, nonempty" and
// the same for securityContextId (a call carrying a blank/absent staleness
// check cannot be validated against anything).
func (s Scope) Validate() error {
	if err := requireNonEmpty("project", s.Project); err != nil {
		return err
	}
	if err := requireNonEmpty("environment", s.Environment); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", s.SecurityContextID); err != nil {
		return err
	}
	return nil
}

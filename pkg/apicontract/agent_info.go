package apicontract

import "fmt"

// AgentPrincipal is the effective identity agent-info reports - never the
// role inferred from a name; the server states it explicitly.
type AgentPrincipal struct {
	ID     string   `json:"id"`
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
}

func (p AgentPrincipal) Validate() error {
	return requireNonEmpty("id", p.ID)
}

// AgentProjectRef is one project agent-info reports as registered.
type AgentProjectRef struct {
	ID string `json:"id"`
}

func (r AgentProjectRef) Validate() error {
	return requireNonEmpty("id", r.ID)
}

// AgentCapabilities reports what the session may do - never what it did;
// Result.Provenance.ExecutionProfile reports the actual executor used for a
// given run, "even a privileged principal running protected DTQL still
// receives protected provenance".
type AgentCapabilities struct {
	ProtectedQueries bool `json:"protectedQueries"`
	OpaqueReadOnly   bool `json:"opaqueReadOnly"`
}

// AgentInfo is the exact success envelope of GET agent-info.
// api-contract.md "Endpoint table".
type AgentInfo struct {
	Version           string            `json:"version"`
	Principal         AgentPrincipal    `json:"principal"`
	SecurityContextID string            `json:"securityContextId"`
	Projects          []AgentProjectRef `json:"projects"`
	Capabilities      AgentCapabilities `json:"capabilities"`
}

// Validate enforces Version and SecurityContextID are required, Principal is
// itself valid, and every Projects entry is itself valid.
func (a AgentInfo) Validate() error {
	if err := requireNonEmpty("version", a.Version); err != nil {
		return err
	}
	if err := a.Principal.Validate(); err != nil {
		return err
	}
	if err := requireNonEmpty("securityContextId", a.SecurityContextID); err != nil {
		return err
	}
	for i, p := range a.Projects {
		if err := p.Validate(); err != nil {
			return &ValidationError{Field: "projects", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

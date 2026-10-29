package dto

// PublicOVDBConnectionState describes a bounded, native declaration capability.
// Source URLs and credentials are not part of this contract.
type PublicOVDBConnectionState struct {
	Contract      string           `json:"contract"`
	Scope         EnvironmentScope `json:"scope"`
	ScopeBinding  string           `json:"scopeBinding"`
	Revision      string           `json:"revision"`
	EnvironmentID string           `json:"environmentId"`
	DatabaseID    string           `json:"databaseId"`
	ConnectionID  string           `json:"connectionId"`
	CanAdd        bool             `json:"canAdd"`
	Reason        string           `json:"reason,omitempty"`
}

type CreatePublicOVDBConnectionRequest struct {
	EnvironmentMutation
	EnvironmentID string `json:"environmentId"`
	DatabaseID    string `json:"databaseId"`
}

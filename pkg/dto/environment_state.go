package dto

import "encoding/json"

// EnvironmentScope preserves requested qualifiers alongside resolved authority.
type EnvironmentScope struct {
	StoreID        string  `json:"storeId"`
	ProjectID      string  `json:"projectId"`
	SpaceID        *string `json:"spaceID,omitempty"`
	ProjectAPI     *string `json:"projectApi,omitempty"`
	Branch         *string `json:"branch,omitempty"`
	AuthorityID    string  `json:"authorityId"`
	ResolvedBranch *string `json:"resolvedBranch,omitempty"`
	RepositoryID   *int64  `json:"repositoryId,omitempty"`
	Owner          *string `json:"owner,omitempty"`
	Repository     *string `json:"repository,omitempty"`
	Folder         *string `json:"folder,omitempty"`
}
type EnvironmentPresentation struct {
	Label string `json:"label,omitempty"`
	Icon  string `json:"icon,omitempty"`
	Tone  string `json:"tone,omitempty"`
}

// EnvironmentBrief excludes connection and server declarations.
type EnvironmentBrief struct {
	ID           string                   `json:"id"`
	Title        string                   `json:"title,omitempty"`
	Access       string                   `json:"access,omitempty"`
	Presentation *EnvironmentPresentation `json:"presentation,omitempty"`
}
type EnvironmentCapabilities struct {
	CanReadOrder         bool              `json:"canReadOrder"`
	CanWriteOrder        bool              `json:"canWriteOrder"`
	CanCreateEnvironment bool              `json:"canCreateEnvironment"`
	Reasons              map[string]string `json:"reasons,omitempty"`
}
type EnvironmentState struct {
	Contract     string                  `json:"contract"`
	Scope        EnvironmentScope        `json:"scope"`
	ScopeBinding string                  `json:"scopeBinding"`
	Environments []EnvironmentBrief      `json:"environments"`
	SavedOrder   []string                `json:"savedOrder"`
	Order        []string                `json:"order"`
	NeedsRepair  bool                    `json:"needsRepair"`
	Revision     string                  `json:"revision"`
	Capabilities EnvironmentCapabilities `json:"capabilities"`
}
type EnvironmentMutation struct {
	Scope       EnvironmentScope `json:"scope"`
	IfRevision  string           `json:"ifRevision"`
	OperationID string           `json:"operationId"`
}
type CreateEnvironmentRequest struct {
	EnvironmentMutation
	Name string `json:"name"`
}
type SaveEnvironmentOrderRequest struct {
	EnvironmentMutation
	EnvironmentOrder []string `json:"environmentOrder"`
}
type EnvironmentOperationStatusRequest struct {
	Scope       EnvironmentScope `json:"scope"`
	OperationID string           `json:"operationId"`
}
type EnvironmentOperationResult struct {
	OperationID      string   `json:"operationId"`
	Kind             string   `json:"kind"`
	PayloadDigest    string   `json:"payloadDigest"`
	AcceptedAt       string   `json:"acceptedAt"`
	ReplayUntil      string   `json:"replayUntil"`
	EnvironmentID    string   `json:"environmentId,omitempty"`
	EnvironmentOrder []string `json:"environmentOrder,omitempty"`
	AfterStateDigest string   `json:"afterStateDigest"`
	Status           string   `json:"status"`
}

// MarshalJSON preserves an accepted empty order on a save receipt.
func (v EnvironmentOperationResult) MarshalJSON() ([]byte, error) {
	type plain EnvironmentOperationResult
	body, err := json.Marshal(plain(v))
	if v.EnvironmentOrder == nil || len(v.EnvironmentOrder) > 0 {
		return body, err
	}
	var fields map[string]json.RawMessage
	// The static plain receipt contains only strings and string slices; its
	// generated JSON is always an object and cannot fail this decode.
	_ = json.Unmarshal(body, &fields)
	fields["environmentOrder"] = json.RawMessage("[]")
	return json.Marshal(fields)
}

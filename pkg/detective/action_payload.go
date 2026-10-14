package detective

import (
	"encoding/hex"
	"strings"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// ActionOperationPayload includes trusted bridge metadata in operation digests
// even though ordinary action JSON cannot bind these fields. Adapters must call
// this before BindOperation and freshly resolve/authorize the private reference.
// Nil-reference legacy actions retain their exact previous JSON payload.
// The returned payload contains private inputs for hashing, not receipt/audit storage.
func ActionOperationPayload(req ActionRequest) (any, error) {
	ref, err := canonicalActionInputReference(req.InputReference, req.ExpectedDefinitionDigest, req.ExpectedCaseContextDigest, req.CaseID, req.Environment, req.Inputs)
	if err != nil {
		return nil, err
	}
	if ref == nil {
		return req, nil
	}
	if req.Type != "run-check" || !validSlug(req.CheckID) || strings.TrimSpace(req.Project) == "" || req.QueryID != "" || req.Execution != nil || req.Check != nil || req.Validator != nil || req.QueryValidators != nil || req.Case != nil || req.Scenario != nil || req.Event != nil || req.ScenarioID != "" || req.ApplicationID != "" || req.StepID != "" || req.StepStatus != "" || req.RunID != "" {
		return nil, ErrInvalid
	}
	owned, err := cloneValue(req)
	if err != nil {
		return nil, err
	}
	return struct {
		BridgeSchemaVersion       int                                `json:"bridgeSchemaVersion"`
		Action                    ActionRequest                      `json:"action"`
		InputReference            *apicontract.SessionInputReference `json:"inputReference"`
		ExpectedDefinitionDigest  string                             `json:"expectedDefinitionDigest"`
		ExpectedCaseContextDigest string                             `json:"expectedCaseContextDigest,omitempty"`
	}{1, owned, ref, req.ExpectedDefinitionDigest, req.ExpectedCaseContextDigest}, nil
}

func canonicalActionInputReference(ref *apicontract.SessionInputReference, definitionDigest, caseDigest, caseID, environment string, inputs map[string]InputValue) (*apicontract.SessionInputReference, error) {
	if ref == nil {
		if definitionDigest != "" || caseDigest != "" {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	owned, err := ref.Canonical()
	if err != nil || definitionDigest != ref.DefinitionDigest || environment != ref.EnvironmentID || len(ref.Bindings) != len(inputs) {
		return nil, ErrInvalid
	}
	if caseID == "" {
		if caseDigest != "" {
			return nil, ErrInvalid
		}
	} else {
		digest, err := hex.DecodeString(caseDigest)
		if !validSlug(caseID) || err != nil || len(digest) != 32 || strings.ToLower(caseDigest) != caseDigest {
			return nil, ErrInvalid
		}
	}
	for _, binding := range owned.Bindings {
		if _, ok := inputs[binding.InputName]; !ok || binding.Origin == "case" && binding.CaseID != caseID {
			return nil, ErrInvalid
		}
	}
	return &owned, nil
}

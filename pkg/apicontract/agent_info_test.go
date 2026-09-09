package apicontract

import (
	"encoding/json"
	"testing"
)

func validAgentInfo() AgentInfo {
	return AgentInfo{
		Version:           "0.25.0",
		Principal:         AgentPrincipal{ID: "boss", Roles: []string{"admin"}, Groups: []string{}},
		SecurityContextID: "sc1",
		Projects:          []AgentProjectRef{{ID: "demo-project-1"}},
		Capabilities:      AgentCapabilities{ProtectedQueries: true, OpaqueReadOnly: true},
	}
}

func TestAgentInfo_JSONFieldNames(t *testing.T) {
	a := validAgentInfo()
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":"0.25.0","principal":{"id":"boss","roles":["admin"],"groups":[]},"securityContextId":"sc1","projects":[{"id":"demo-project-1"}],"capabilities":{"protectedQueries":true,"opaqueReadOnly":true}}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}
}

func TestAgentInfo_EmptyRolesGroupsProjectsMarshalAsEmptyArrays(t *testing.T) {
	a := AgentInfo{
		Version:           "0.25.0",
		Principal:         AgentPrincipal{ID: "anon", Roles: []string{}, Groups: []string{}},
		SecurityContextID: "sc1",
		Projects:          []AgentProjectRef{},
		Capabilities:      AgentCapabilities{},
	}
	data, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]json.RawMessage
	_ = json.Unmarshal(data, &generic)
	if string(generic["projects"]) != "[]" {
		t.Errorf("projects = %s, want []", generic["projects"])
	}
}

func TestAgentInfo_Validate(t *testing.T) {
	if err := validAgentInfo().Validate(); err != nil {
		t.Errorf("expected valid, got: %v", err)
	}

	missingVersion := validAgentInfo()
	missingVersion.Version = ""
	if err := missingVersion.Validate(); err == nil {
		t.Error("expected an error: missing version")
	}

	missingPrincipalID := validAgentInfo()
	missingPrincipalID.Principal.ID = ""
	if err := missingPrincipalID.Validate(); err == nil {
		t.Error("expected an error: missing principal.id")
	}

	missingSecurityContext := validAgentInfo()
	missingSecurityContext.SecurityContextID = ""
	if err := missingSecurityContext.Validate(); err == nil {
		t.Error("expected an error: missing securityContextId")
	}

	badProjectRef := validAgentInfo()
	badProjectRef.Projects = []AgentProjectRef{{}}
	if err := badProjectRef.Validate(); err == nil {
		t.Error("expected an error: project ref with no id")
	}
}

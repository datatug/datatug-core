package apicontract

import (
	"encoding/json"
	"testing"
)

func TestScope_JSONFieldNames(t *testing.T) {
	s := Scope{Project: "p1", Environment: "local", SecurityContextID: "sc1"}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"project":"p1","environment":"local","securityContextId":"sc1"}`
	if string(data) != want {
		t.Errorf("got %s, want %s", data, want)
	}

	var back Scope
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if back != s {
		t.Errorf("round-trip mismatch: got %+v, want %+v", back, s)
	}
}

func TestScope_Validate(t *testing.T) {
	valid := Scope{Project: "p1", Environment: "local", SecurityContextID: "sc1"}
	if err := valid.Validate(); err != nil {
		t.Errorf("expected a valid scope, got error: %v", err)
	}

	cases := []struct {
		name  string
		scope Scope
	}{
		{"missing project", Scope{Environment: "local", SecurityContextID: "sc1"}},
		{"missing environment", Scope{Project: "p1", SecurityContextID: "sc1"}},
		{"missing securityContextId", Scope{Project: "p1", Environment: "local"}},
		{"blank project", Scope{Project: "  ", Environment: "local", SecurityContextID: "sc1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := c.scope.Validate(); err == nil {
				t.Errorf("expected an error for %+v", c.scope)
			}
		})
	}
}

func TestScope_StoreIDIsOptionalButValidated(t *testing.T) {
	withoutStore := Scope{Project: "demo", Environment: "default", SecurityContextID: "ctx-1"}
	if err := withoutStore.Validate(); err != nil {
		t.Fatalf("primary-store scope should remain valid: %v", err)
	}
	withStore := withoutStore
	withStore.StoreID = "ops"
	if err := withStore.Validate(); err != nil {
		t.Fatalf("qualified scope should be valid: %v", err)
	}
	withStore.StoreID = "bad/store"
	if err := withStore.Validate(); err == nil {
		t.Fatal("store id containing slash should be rejected")
	}
	withStore.StoreID = " ops"
	if err := withStore.Validate(); err == nil {
		t.Fatal("store id with surrounding whitespace should be rejected")
	}
}

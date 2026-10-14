package detective

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func bridgeActionFixture() ActionRequest {
	return ActionRequest{Type: "run-check", Project: "native-project", SecurityContextID: "security-1", CheckID: "root", CaseID: "case-1", Environment: "qa",
		InputReference: &apicontract.SessionInputReference{SchemaVersion: 1, StorageID: "firestore", SpaceID: "space-1", DatabaseID: "private-db", ProjectID: "private-project", NamespaceID: "namespace-1", SessionID: "session-1", InputSnapshotID: "input-1", ContextSetID: "context-1", EnvironmentID: "qa", IdentityDomain: "identity-1", DefinitionDigest: strings.Repeat("a", 64), Bindings: []apicontract.SessionInputBindingRef{
			{InputName: "InvoiceId", Origin: "bag", Entries: []apicontract.SessionInputEntryRef{{BagID: "bag-1", BagVersion: 2, EntryID: "entry-2", RevisionID: "revision-2"}, {BagID: "bag-1", BagVersion: 2, EntryID: "entry-1", RevisionID: "revision-1"}}},
			{InputName: "CustomerId", Origin: "case", CaseID: "case-1", CaseVersion: 3},
		}}, ExpectedDefinitionDigest: strings.Repeat("a", 64), ExpectedCaseContextDigest: strings.Repeat("b", 64),
		Inputs: map[string]InputValue{"InvoiceId": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}, "CustomerId": {Scalar: ptrValue(investigation.NewIntegerValue("2"))}},
	}
}

func bridgeBinding(t *testing.T, req ActionRequest) OperationBinding {
	t.Helper()
	payload, err := ActionOperationPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := BindOperation(req.Project, "alice", req.Type, req.CheckID, payload)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestActionOperationPayloadLegacyFixture(t *testing.T) {
	req := ActionRequest{Type: "run-check", Project: "native-project", SecurityContextID: "security-1", CheckID: "root", CaseID: "case-1", Environment: "qa", Inputs: map[string]InputValue{"InvoiceId": {Scalar: ptrValue(investigation.NewIntegerValue("1"))}}}
	const legacyJSON = `{"type":"run-check","project":"native-project","securityContextId":"security-1","checkId":"root","caseId":"case-1","environment":"qa","inputs":{"InvoiceId":{"type":"integer","value":"1"}}}`
	payload, err := ActionOperationPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(payload)
	if err != nil || string(data) != legacyJSON {
		t.Fatalf("legacy JSON changed: %s %v", data, err)
	}
	old, err := BindOperation(req.Project, "alice", req.Type, req.CheckID, req)
	if err != nil {
		t.Fatal(err)
	}
	current := bridgeBinding(t, req)
	// Captures the existing BindOperation scope struct order plus sorted payload maps.
	const legacyDigest = "5afdd08f7c31a24ff92584f084592ec050daf691b2ec5e658b7128e32356a712"
	if current.PayloadDigest != legacyDigest {
		t.Fatalf("legacy fixture digest changed: %s", current.PayloadDigest)
	}
	if current != old {
		t.Fatalf("legacy digest changed: %+v %+v", current, old)
	}
	var fromFixture ActionRequest
	if err := apicontract.DecodeStrict([]byte(legacyJSON), &fromFixture); err != nil {
		t.Fatal(err)
	}
	if bridgeBinding(t, fromFixture) != current {
		t.Fatal("fixture replay changed")
	}
	legacyRun := CheckRun{ID: "run-1", InputNames: []string{"InvoiceId"}}
	data, _ = json.Marshal(legacyRun)
	if strings.Contains(string(data), "inputReference") {
		t.Fatal("absent optional reference changes legacy wire form")
	}
}

func TestActionOperationPayloadBindsCanonicalOwnedReference(t *testing.T) {
	req := bridgeActionFixture()
	payload, err := ActionOperationPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(payload)
	binding := bridgeBinding(t, req)
	reordered := bridgeActionFixture()
	reordered.InputReference.Bindings[0], reordered.InputReference.Bindings[1] = reordered.InputReference.Bindings[1], reordered.InputReference.Bindings[0]
	reordered.InputReference.Bindings[1].Entries[0], reordered.InputReference.Bindings[1].Entries[1] = reordered.InputReference.Bindings[1].Entries[1], reordered.InputReference.Bindings[1].Entries[0]
	if bridgeBinding(t, reordered) != binding {
		t.Fatal("ordering changes semantic digest")
	}
	req.InputReference.Bindings[0].Entries[0].RevisionID = "changed"
	req.InputReference.Bindings[1].CaseID = "changed"
	req.Inputs["InvoiceId"].Scalar.Str = "PRIVATE_SENTINEL"
	after, _ := json.Marshal(payload)
	if string(before) != string(after) {
		t.Fatal("accepted digest payload aliases request")
	}
	var envelope struct {
		InputReference apicontract.SessionInputReference `json:"inputReference"`
	}
	if err := json.Unmarshal(before, &envelope); err != nil {
		t.Fatal(err)
	}
	refJSON, _ := json.Marshal(envelope.InputReference)
	if strings.Contains(string(refJSON), `"value"`) {
		t.Fatal("input values copied to metadata reference")
	}
	caseFree := bridgeActionFixture()
	caseFree.CaseID, caseFree.ExpectedCaseContextDigest = "", ""
	caseFree.InputReference.Bindings = caseFree.InputReference.Bindings[:1]
	delete(caseFree.Inputs, "CustomerId")
	if _, err := ActionOperationPayload(caseFree); err != nil {
		t.Fatal("Case-free bridge rejected:", err)
	}
}

func TestActionOperationPayloadEveryIdentityChangesDigest(t *testing.T) {
	base := bridgeBinding(t, bridgeActionFixture()).PayloadDigest
	for _, field := range []string{"StorageID", "SpaceID", "DatabaseID", "ProjectID", "NamespaceID", "SessionID", "InputSnapshotID", "ContextSetID", "IdentityDomain"} {
		t.Run(field, func(t *testing.T) {
			req := bridgeActionFixture()
			reflect.ValueOf(req.InputReference).Elem().FieldByName(field).SetString("other")
			if bridgeBinding(t, req).PayloadDigest == base {
				t.Fatal("reference identity not bound")
			}
		})
	}
	for _, tc := range []struct {
		name   string
		change func(*ActionRequest)
	}{
		{"definition", func(r *ActionRequest) {
			r.ExpectedDefinitionDigest = strings.Repeat("c", 64)
			r.InputReference.DefinitionDigest = r.ExpectedDefinitionDigest
		}},
		{"environment", func(r *ActionRequest) { r.Environment = "prod"; r.InputReference.EnvironmentID = "prod" }},
		{"native project", func(r *ActionRequest) { r.Project = "other-project" }},
		{"native Check", func(r *ActionRequest) { r.CheckID = "other-check" }},
		{"Case", func(r *ActionRequest) { r.CaseID = "other-case"; r.InputReference.Bindings[1].CaseID = r.CaseID }},
		{"Case digest", func(r *ActionRequest) { r.ExpectedCaseContextDigest = strings.Repeat("c", 64) }},
		{"Case version", func(r *ActionRequest) { r.InputReference.Bindings[1].CaseVersion++ }},
		{"binding name", func(r *ActionRequest) {
			r.InputReference.Bindings[0].InputName = "OtherId"
			r.Inputs["OtherId"] = r.Inputs["InvoiceId"]
			delete(r.Inputs, "InvoiceId")
		}},
		{"binding origin", func(r *ActionRequest) {
			r.InputReference.Bindings[0].Origin = "manual"
			r.InputReference.Bindings[0].Entries = nil
		}},
		{"bag", func(r *ActionRequest) {
			for i := range r.InputReference.Bindings[0].Entries {
				r.InputReference.Bindings[0].Entries[i].BagID = "other-bag"
			}
		}},
		{"bag version", func(r *ActionRequest) {
			for i := range r.InputReference.Bindings[0].Entries {
				r.InputReference.Bindings[0].Entries[i].BagVersion++
			}
		}},
		{"entry", func(r *ActionRequest) { r.InputReference.Bindings[0].Entries[0].EntryID = "other-entry" }},
		{"revision", func(r *ActionRequest) { r.InputReference.Bindings[0].Entries[0].RevisionID = "other-revision" }},
		{"value", func(r *ActionRequest) { r.Inputs["InvoiceId"].Scalar.Str = "4" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := bridgeActionFixture()
			tc.change(&req)
			if bridgeBinding(t, req).PayloadDigest == base {
				t.Fatal("semantic request field not bound")
			}
		})
	}
}

func TestActionOperationPayloadRejectsForgedOrUnsupportedBridge(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*ActionRequest)
	}{
		{"nil ref digest", func(r *ActionRequest) { r.InputReference = nil }},
		{"nil ref Case digest", func(r *ActionRequest) { r.InputReference = nil; r.ExpectedDefinitionDigest = "" }},
		{"invalid ref", func(r *ActionRequest) { r.InputReference.NamespaceID = "../bad" }},
		{"definition mismatch", func(r *ActionRequest) { r.ExpectedDefinitionDigest = strings.Repeat("c", 64) }},
		{"environment mismatch", func(r *ActionRequest) { r.Environment = "prod" }},
		{"missing input", func(r *ActionRequest) { delete(r.Inputs, "CustomerId") }},
		{"other input", func(r *ActionRequest) { r.Inputs["OtherId"] = r.Inputs["CustomerId"]; delete(r.Inputs, "CustomerId") }},
		{"Case mismatch", func(r *ActionRequest) { r.CaseID = "other-case" }},
		{"no Case digest", func(r *ActionRequest) { r.ExpectedCaseContextDigest = "" }},
		{"short Case digest", func(r *ActionRequest) { r.ExpectedCaseContextDigest = "ab" }},
		{"bad Case digest", func(r *ActionRequest) { r.ExpectedCaseContextDigest = strings.Repeat("z", 64) }},
		{"uppercase Case digest", func(r *ActionRequest) { r.ExpectedCaseContextDigest = strings.Repeat("B", 64) }},
		{"invalid Case", func(r *ActionRequest) { r.CaseID = "../case" }},
		{"Case digest without Case", func(r *ActionRequest) { r.CaseID = "" }},
		{"action", func(r *ActionRequest) { r.Type = "run-query-validators" }},
		{"invalid Check", func(r *ActionRequest) { r.CheckID = "../check" }},
		{"missing project", func(r *ActionRequest) { r.Project = " " }},
		{"query", func(r *ActionRequest) { r.QueryID = "q" }},
		{"execution", func(r *ActionRequest) { r.Execution = &apicontract.ExecutionRef{} }},
		{"authored Check", func(r *ActionRequest) { r.Check = &Check{} }},
		{"authored validator", func(r *ActionRequest) { r.Validator = &PublishedValidator{} }},
		{"authored locals", func(r *ActionRequest) { r.QueryValidators = &QueryValidators{} }},
		{"authored Case", func(r *ActionRequest) { r.Case = &Case{} }},
		{"authored scenario", func(r *ActionRequest) { r.Scenario = &Scenario{} }},
		{"authored event", func(r *ActionRequest) { r.Event = &CaseEvent{} }},
		{"scenario", func(r *ActionRequest) { r.ScenarioID = "scenario" }},
		{"application", func(r *ActionRequest) { r.ApplicationID = "application" }},
		{"step", func(r *ActionRequest) { r.StepID = "step" }},
		{"step status", func(r *ActionRequest) { r.StepStatus = "completed" }},
		{"run", func(r *ActionRequest) { r.RunID = "run" }},
		{"invalid value encoding", func(r *ActionRequest) { r.Inputs["InvoiceId"] = InputValue{} }},
		{"invalid value decoding", func(r *ActionRequest) { r.Inputs["InvoiceId"].Scalar.Str = "invalid-integer" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := bridgeActionFixture()
			tc.change(&req)
			if _, err := ActionOperationPayload(req); !errors.Is(err, ErrInvalid) {
				t.Fatalf("invalid bridge accepted: %v", err)
			}
		})
	}
	req := bridgeActionFixture()
	data, _ := json.Marshal(req)
	for _, field := range []string{"inputReference", "expectedDefinitionDigest", "expectedCaseContextDigest"} {
		if strings.Contains(string(data), field) {
			t.Fatal("internal field serialized in public action")
		}
		forged := append(append([]byte(nil), data[:len(data)-1]...), []byte(`,"`+field+`":null}`)...)
		var decoded ActionRequest
		if apicontract.DecodeStrict(forged, &decoded) == nil {
			t.Fatalf("client can bind %s", field)
		}
	}
	var decoded ActionRequest
	if err := apicontract.DecodeStrict(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.InputReference != nil || decoded.ExpectedDefinitionDigest != "" || decoded.ExpectedCaseContextDigest != "" {
		t.Fatal("trusted data survived public JSON")
	}
}

func TestEvaluateCheckOwnsTrustedReferenceAndFailsBeforeSource(t *testing.T) {
	ctx := context.Background()
	check := Check{ID: "root", Title: "Root", Inputs: []Input{{Name: "InvoiceId", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}, {Name: "CustomerId", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "q"}}}
	reader := mapReader{checks: map[string]Check{"root": check}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(ctx, "root", "", reader)
	if err != nil {
		t.Fatal(err)
	}
	bridge := bridgeActionFixture()
	req := RunRequest{ID: "run-1", CheckID: "root", CaseID: bridge.CaseID, Environment: bridge.Environment, Inputs: bridge.Inputs, InputReference: bridge.InputReference, ExpectedDefinitionDigest: bridge.ExpectedDefinitionDigest, ExpectedCaseContextDigest: bridge.ExpectedCaseContextDigest}
	calls := 0
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls++
		bridge.InputReference.Bindings[0].Entries[0].RevisionID = "changed-during-execution"
		return coreResult([]string{"x"}, nil), nil
	}
	run, err := EvaluateCheck(ctx, req, catalog, runner, nil)
	if err != nil || calls != 1 {
		t.Fatalf("run error=%v calls=%d", err, calls)
	}
	if run.InputReference.Bindings[1].Entries[1].RevisionID != "revision-2" {
		t.Fatal("run reference aliases dispatch caller")
	}
	copy, err := cloneValue(run)
	if err != nil {
		t.Fatal(err)
	}
	run.InputReference.Bindings[1].Entries[0].RevisionID = "changed-after-run"
	if copy.InputReference.Bindings[1].Entries[0].RevisionID != "revision-1" {
		t.Fatal("persisted run clone aliases reference")
	}
	req.ExpectedDefinitionDigest = "bad"
	if _, err := EvaluateCheck(ctx, req, catalog, runner, nil); !errors.Is(err, ErrInvalid) || calls != 1 {
		t.Fatal("bad trusted ref reached source")
	}
	queryCatalog, err := LoadCatalog(ctx, "", "q", reader)
	if err != nil {
		t.Fatal(err)
	}
	req.QueryID, req.CheckID = "q", ""
	if _, err := EvaluateQueryValidators(ctx, req, queryCatalog, runner, nil); !errors.Is(err, ErrInvalid) || calls != 1 {
		t.Fatal("query-validator accepted Check bridge")
	}
}

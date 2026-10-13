package detectiveai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/qri-io/jsonschema"
	"github.com/strongo/aichat/ai"
	"github.com/strongo/aichat/ai/anthropic"
	"github.com/strongo/aichat/ai/openaicompat"
	"github.com/strongo/aichat/ai/openairesponses"
)

func TestProviderReceivesCompleteNestedProposalSchema(t *testing.T) {
	scope, snapshot, sources := baseline()
	provider := &fakeProvider{steps: [][]ai.Event{{{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	if _, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), Request{Scope: scope, Snapshot: snapshot, Text: "Find invoice 7", Sources: sources}); err != nil {
		t.Fatal(err)
	}
	if len(provider.seen) != 1 || len(provider.seen[0].Tools) != 2 || provider.seen[0].Tools[1].Name != "submit_proposal" {
		t.Fatalf("provider did not receive proposal tool: %+v", provider.seen)
	}
	var schema jsonschema.Schema
	if err := json.Unmarshal(provider.seen[0].Tools[1].Schema, &schema); err != nil {
		t.Fatalf("provider schema is not valid JSON Schema: %v", err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(provider.seen[0].Tools[1].Schema, &root); err != nil || string(root["type"]) != `"object"` {
		t.Fatalf("provider tool needs a native object root: %s, %v", root["type"], err)
	}
	accept := func(name string, draft Draft) {
		t.Helper()
		if err := validateDraft(draft, snapshot, sources); err != nil {
			t.Fatalf("%s is not a valid Core proposal: %v", name, err)
		}
		encoded, err := json.Marshal(draft)
		if err != nil {
			t.Fatalf("%s marshal: %v", name, err)
		}
		issues, err := schema.ValidateBytes(context.Background(), encoded)
		if err != nil || len(issues) != 0 {
			t.Fatalf("provider schema rejected %s: %v, %v; payload=%s", name, issues, err, encoded)
		}
	}
	for _, draft := range typedDrafts() {
		accept(draft.Kind, draft)
	}
	parameterized := typedDrafts()[2]
	parameterized.Check.Inputs = []detective.Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	parameterized.Check.Steps[0].Bindings = []detective.Binding{{Input: "invoice_id", FromInput: "invoice_id"}}
	parameterized.Check.Steps = append(parameterized.Check.Steps, detective.Step{ID: "compare-detail", Kind: "check", CheckID: "invoice-check", Bindings: []detective.Binding{{Input: "invoice_id", FromInput: "invoice_id"}}})
	accept("parameterized query and child Check", parameterized)
	withLocalCheckValidator := typedDrafts()[2]
	withLocalCheckValidator.Check.Steps[0].Validators = []detective.Validator{{ID: "nested", Kind: "check", Scope: "row", Severity: "critical", CheckID: "invoice-check", Bindings: []detective.Binding{{Input: "invoice_id", FromColumn: "id"}}, OnUnavailable: "fail"}}
	accept("query-local nested Check Validator", withLocalCheckValidator)
	snapshot.Validators = []Resource{{ID: "required-id", Title: "Required ID", Revision: "1"}}
	withCatalogValidator := typedDrafts()[2]
	withCatalogValidator.Check.Steps[0].Validators = []detective.Validator{{ID: "reuse", ValidatorID: "required-id"}}
	accept("promoted Validator reference", withCatalogValidator)
	updatedQuery := typedDrafts()[1]
	updatedQuery.Query.ID = "invoice-list"
	updatedQuery.Query.ExpectedRevision = "q-3"
	updatedQuery.Query.ConnectionID = "source-1"
	accept("query revision and connection", updatedQuery)
	caseEdit := typedDrafts()[5]
	caseEdit.Case.Findings[0].EvidenceRefs = []string{"run-a"}
	caseEdit.Case.Status = "closed"
	caseEdit.Case.ClosureReason = "Reviewed by analyst"
	accept("Case edit with evidence and closure", caseEdit)
	snapshot.Checks = append(snapshot.Checks, Resource{ID: "no-input-check", Title: "No inputs", Revision: "1"})
	noInputRun := runDraft()
	noInputRun.Run.CheckID = "no-input-check"
	noInputRun.Run.Inputs = nil
	accept("run without inputs", noInputRun)
	first, second := investigation.NewIntegerValue("7"), investigation.NewIntegerValue("8")
	set, err := apicontract.NewTypedValueSet(first, second)
	if err != nil {
		t.Fatal(err)
	}
	accept("set extraction", Draft{Kind: KindContext, Title: "Two invoice IDs", Context: map[string]ExtractedValue{"invoice_ids": {Value: detective.InputValue{Set: &set}, Provenance: cited()}}, Sources: cited()})
	accept("range extraction", Draft{Kind: KindContext, Title: "Invoice range", Context: map[string]ExtractedValue{"invoice_range": {Value: detective.InputValue{Range: &detective.ValueRange{Type: "range", Start: first, End: second}}, Provenance: cited()}}, Sources: cited()})
	for name, raw := range map[string]string{
		"missing query text":            `{"kind":"draft-query","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"query":{"id":"q","title":"q","type":"SQL"}}`,
		"wrong scalar":                  `{"kind":"extract-context","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"context":{"invoice_id":{"value":{"type":"integer","value":7},"provenance":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}]}}}`,
		"set lacks typed values":        `{"kind":"extract-context","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"context":{"invoice_ids":{"value":{"type":"set","values":["7","8"]},"provenance":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}]}}}`,
		"range lacks end":               `{"kind":"extract-context","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"context":{"invoice_range":{"value":{"type":"range","start":{"type":"integer","value":"7"}},"provenance":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}]}}}`,
		"missing citation location":     `{"kind":"run-check","title":"x","sources":[{"sourceId":"ticket","confidence":0.8}],"run":{"checkId":"invoice-check","environment":"local","inputs":{}}}`,
		"two action kinds":              `{"kind":"run-check","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"run":{"checkId":"invoice-check","environment":"local","inputs":{}},"query":{"id":"q","title":"q","type":"SQL","text":"select 1"}}`,
		"missing check steps":           `{"kind":"draft-check","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"check":{"id":"c","title":"c"}}`,
		"child check missing reference": `{"kind":"draft-check","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"check":{"id":"c","title":"c","steps":[{"id":"s","kind":"check"}]}}`,
		"validator missing field":       `{"kind":"draft-validator","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"validator":{"id":"v","title":"v","validator":{"id":"local","kind":"non-null","scope":"row","severity":"error"}}}`,
		"case missing version":          `{"kind":"edit-case","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"case":{"caseId":"case-a"}}`,
		"incident authority spoof":      `{"kind":"edit-case","title":"x","sources":[{"sourceId":"ticket","start":0,"end":9,"confidence":0.8}],"case":{"caseId":"case-a","expectedVersion":3,"incidentRefs":["other"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			issues, err := schema.ValidateBytes(context.Background(), []byte(raw))
			if err != nil || len(issues) == 0 {
				t.Fatalf("provider schema accepted bad nested payload: %v, %v", issues, err)
			}
		})
	}
	if len(provider.seen[0].Tools[1].Schema) < 3000 || strings.Contains(string(provider.seen[0].Tools[1].Schema), `"case":{"type":"object"}`) {
		t.Fatal("provider received shallow payload schemas")
	}
}

type capturedToolRequest struct{ body []byte }

func (r *capturedToolRequest) RoundTrip(request *http.Request) (*http.Response, error) {
	r.body, _ = io.ReadAll(request.Body)
	return &http.Response{StatusCode: http.StatusBadRequest, Body: io.NopCloser(strings.NewReader(`{"error":{"message":"test capture"}}`)), Header: http.Header{"Content-Type": []string{"application/json"}}, Request: request}, nil
}

func TestProviderAdaptersSendNativeObjectToolSchemas(t *testing.T) {
	scope, snapshot, sources := baseline()
	for _, tc := range []struct {
		name     string
		path     []string
		provider func(*http.Client) ai.LLMProvider
	}{
		{"Anthropic input_schema", []string{"tools", "input_schema"}, func(client *http.Client) ai.LLMProvider {
			return anthropic.New(anthropic.Config{BaseURL: "https://example.invalid", APIKey: "test", HTTPClient: client})
		}},
		{"OpenAI Chat parameters", []string{"tools", "function", "parameters"}, func(client *http.Client) ai.LLMProvider {
			return openaicompat.New(openaicompat.Config{BaseURL: "https://example.invalid", APIKey: "test", Model: "gpt-4o-mini", HTTPClient: client})
		}},
		{"OpenAI Responses parameters", []string{"tools", "parameters"}, func(client *http.Client) ai.LLMProvider {
			return openairesponses.New(openairesponses.Config{BaseURL: "https://example.invalid", APIKey: "test", Model: "gpt-4o-mini", HTTPClient: client})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			capture := &capturedToolRequest{}
			_, _ = (Assistant{Provider: tc.provider(&http.Client{Transport: capture}), Store: &memoryStore{}}).Run(context.Background(), Request{Scope: scope, Snapshot: snapshot, Text: "Investigate", Sources: sources})
			if len(capture.body) == 0 {
				t.Fatal("provider did not serialize an outbound request")
			}
			var wire struct {
				Tools []map[string]json.RawMessage `json:"tools"`
			}
			if err := json.Unmarshal(capture.body, &wire); err != nil || len(wire.Tools) != 2 {
				t.Fatalf("native tools missing: %v, %s", err, capture.body)
			}
			var actual json.RawMessage
			switch len(tc.path) {
			case 2:
				actual = wire.Tools[1][tc.path[1]]
			case 3:
				var function map[string]json.RawMessage
				if err := json.Unmarshal(wire.Tools[1][tc.path[1]], &function); err != nil {
					t.Fatal(err)
				}
				actual = function[tc.path[2]]
			}
			var schema map[string]json.RawMessage
			if err := json.Unmarshal(actual, &schema); err != nil || string(schema["type"]) != `"object"` || len(schema["oneOf"]) == 0 || len(schema["$defs"]) == 0 {
				t.Fatalf("native proposal schema lacks object root or nested definitions: %v, %s", err, actual)
			}
		})
	}
}

func TestProposalSchemaCoversEditableCoreWireFields(t *testing.T) {
	var root map[string]any
	if err := json.Unmarshal(proposalToolSchema, &root); err != nil {
		t.Fatal(err)
	}
	definitions, ok := root["$defs"].(map[string]any)
	if !ok {
		t.Fatal("provider definitions missing")
	}
	for _, tc := range []struct {
		name   string
		schema any
		model  any
	}{
		{"draft", root, Draft{}},
		{"provenance", definitions["provenance"], Provenance{}},
		{"extractedValue", definitions["extractedValue"], ExtractedValue{}},
		{"query", definitions["query"], QueryDraft{}},
		{"input", definitions["input"], detective.Input{}},
		{"binding", definitions["binding"], detective.Binding{}},
		{"validator", definitions["validator"], detective.Validator{}},
		{"publishedValidator", definitions["publishedValidator"], detective.PublishedValidator{}},
		{"step", definitions["step"], detective.Step{}},
		{"check", definitions["check"], detective.Check{}},
		{"scenarioStep", definitions["scenarioStep"], detective.ScenarioStep{}},
		{"scenario", definitions["scenario"], detective.Scenario{}},
		{"finding", definitions["finding"], detective.Finding{}},
		{"case", definitions["case"], CaseEdit{}},
		{"run", definitions["run"], RunDraft{}},
		{"explanation", definitions["explanation"], FailureExplanation{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var properties func(any) []string
			properties = func(raw any) []string {
				object, ok := raw.(map[string]any)
				if !ok {
					return nil
				}
				found := []string{}
				if fields, ok := object["properties"].(map[string]any); ok {
					for name := range fields {
						found = append(found, name)
					}
				}
				if branches, ok := object["oneOf"].([]any); ok {
					for _, branch := range branches {
						found = append(found, properties(branch)...)
					}
				}
				slices.Sort(found)
				return slices.Compact(found)
			}
			got := properties(tc.schema)
			typeOf := reflect.TypeOf(tc.model)
			want := make([]string, 0, typeOf.NumField())
			for i := range typeOf.NumField() {
				name := strings.Split(typeOf.Field(i).Tag.Get("json"), ",")[0]
				if name != "" && name != "-" {
					want = append(want, name)
				}
			}
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("schema fields %v differ from editable model fields %v", got, want)
			}
		})
	}
}

package detectiveai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/qri-io/jsonschema"
	"github.com/strongo/aichat/ai"
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
	accept := func(name string, draft Draft) {
		t.Helper()
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
		if draft.Check != nil {
			draft.Check.Inputs = []detective.Input{}
		}
		if draft.Scenario != nil {
			draft.Scenario.Inputs = []detective.Input{}
		}
		accept(draft.Kind, draft)
	}
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

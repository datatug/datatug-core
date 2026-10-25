package detective

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/datatug/datatug-core/pkg/investigation"
)

func TestDefinitionsCreatedAt(t *testing.T) {
	check := Check{ID: "c", Title: "C", Steps: []Step{{ID: "q", Kind: "query", QueryID: "queries/q"}}}
	scenario := Scenario{ID: "s", Title: "S", Inputs: []Input{{Name: "x", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, Steps: []ScenarioStep{{ID: "a", Kind: "query", Ref: "queries/q"}}}
	if err := ValidateCheck(check); err != nil {
		t.Fatal(err)
	}
	if err := ValidateScenario(scenario); err != nil {
		t.Fatal(err)
	}
	check.CreatedAt, scenario.CreatedAt = "2026-10-07T06:18:13Z", "2026-10-07T06:18:13.5Z"
	if err := ValidateCheck(check); err != nil {
		t.Fatal(err)
	}
	if err := ValidateScenario(scenario); err != nil {
		t.Fatal(err)
	}
	for _, v := range []any{check, scenario} {
		data, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var back map[string]any
		if err := json.Unmarshal(data, &back); err != nil || back["createdAt"] == "" || back["createdAt"] == nil {
			t.Fatalf("createdAt lost: %s", data)
		}
	}
	var loaded Scenario
	if err := json.Unmarshal([]byte(`{"id":"s","title":"S","createdAt":"2026-10-07T06:18:13Z"}`), &loaded); err != nil || loaded.CreatedAt == "" {
		t.Fatal(err, loaded)
	}
	check.CreatedAt, scenario.CreatedAt = "nope", "nope"
	if err := ValidateCheck(check); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := ValidateScenario(scenario); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

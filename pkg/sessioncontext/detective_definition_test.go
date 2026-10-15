package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/detective"
)

func definitionFixture() DetectiveCheckDefinitionPin {
	return DetectiveCheckDefinitionPin{SchemaVersion: 1, Check: detective.Check{ID: "root", Title: "Private Check", Version: 1, Steps: []detective.Step{{ID: "child-z", Kind: "check", CheckID: "z"}, {ID: "child-a", Kind: "check", CheckID: "a"}}}, Dependencies: []detective.Check{
		{ID: "z", Title: "Z", Version: 2, Steps: []detective.Step{{ID: "q", Kind: "query", QueryID: "folder/z", Validators: []detective.Validator{{ID: "v", ValidatorID: "vz"}}}}},
		{ID: "a", Title: "A", Version: 3, Steps: []detective.Step{{ID: "q", Kind: "query", QueryID: "folder/a", Validators: []detective.Validator{{ID: "v", ValidatorID: "va"}}}}},
	}, PublishedValidators: []detective.PublishedValidator{{ID: "vz", Title: "Z", Version: 1, Validator: detective.Validator{ID: "z", Kind: "non-null", Scope: "row", Severity: "error", Field: "id"}}, {ID: "va", Title: "A", Version: 1, Validator: detective.Validator{ID: "a", Kind: "non-null", Scope: "row", Severity: "error", Field: "id"}}}, QueryValidators: []detective.QueryValidators{{QueryID: "folder/z", Version: 1, Validators: []detective.Validator{{ID: "local", Kind: "non-null", Scope: "row", Severity: "error", Field: "id"}}}, {QueryID: "folder/a", Version: 1, Validators: []detective.Validator{{ID: "local", Kind: "non-null", Scope: "row", Severity: "error", Field: "id"}}}}, Queries: []DetectiveAcceptedQueryPin{{QueryID: "folder/z", Revision: "z1", DefinitionJSON: json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"SQL","text":"select z"}`)}, {QueryID: "folder/a", Revision: "a1", DefinitionJSON: json.RawMessage(`{"id":"a","folderPath":"folder","title":"A","type":"SQL","text":"select a","parameters":[{"id":"n","type":"number","defaultValue":1.25}]}`)}}, Scenario: &DetectiveScenarioStepIdentity{ScenarioID: "scenario", ScenarioVersion: 1, ApplicationID: "application", StepID: "step"}}
}
func TestCanonicalDetectiveDefinitionOwnsExactExecutableCatalog(t *testing.T) {
	d := definitionFixture()
	owned, digest, err := CanonicalDetectiveDefinition(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	if owned.Dependencies[0].ID != "a" || owned.Queries[0].QueryID != "folder/a" {
		t.Fatal("unordered catalog not sorted")
	}
	swapped := definitionFixture()
	swapped.Dependencies[0], swapped.Dependencies[1] = swapped.Dependencies[1], swapped.Dependencies[0]
	swapped.Queries[0], swapped.Queries[1] = swapped.Queries[1], swapped.Queries[0]
	swapped.PublishedValidators[0], swapped.PublishedValidators[1] = swapped.PublishedValidators[1], swapped.PublishedValidators[0]
	swapped.QueryValidators[0], swapped.QueryValidators[1] = swapped.QueryValidators[1], swapped.QueryValidators[0]
	swapped.Queries[1].DefinitionJSON = json.RawMessage(`{ "text":"select z", "type":"SQL", "title":"Z", "folderPath":"folder", "id":"z" }`)
	_, same, err := CanonicalDetectiveDefinition(context.Background(), swapped)
	if err != nil || same != digest {
		t.Fatalf("canonical order changes digest: %v", err)
	}
	for _, change := range []func(*DetectiveCheckDefinitionPin){func(v *DetectiveCheckDefinitionPin) { v.Queries[0].Revision = "z2" }, func(v *DetectiveCheckDefinitionPin) {
		v.Queries[0].DefinitionJSON = json.RawMessage(strings.ReplaceAll(string(v.Queries[0].DefinitionJSON), "select z", "select changed"))
	}, func(v *DetectiveCheckDefinitionPin) { v.Check.Version++ }, func(v *DetectiveCheckDefinitionPin) { v.Scenario.ApplicationID = "other" }} {
		v := definitionFixture()
		change(&v)
		_, changed, err := CanonicalDetectiveDefinition(context.Background(), v)
		if err != nil || changed == digest {
			t.Fatalf("executable identity omitted: %v", err)
		}
	}
	d.Dependencies[0].Title = "changed"
	d.Queries[1].DefinitionJSON[0] = '!'
	d.Scenario.StepID = "changed"
	if owned.Dependencies[1].Title != "Z" || owned.Scenario.StepID != "step" || !json.Valid(owned.Queries[0].DefinitionJSON) {
		t.Fatal("caller aliases accepted definition")
	}
}

func TestCanonicalDetectiveDefinitionRejectsUnboundedAndInexactSnapshots(t *testing.T) {
	tests := map[string]func(*DetectiveCheckDefinitionPin){
		"schema":          func(v *DetectiveCheckDefinitionPin) { v.SchemaVersion = 0 },
		"version":         func(v *DetectiveCheckDefinitionPin) { v.Check.Version = 0 },
		"scenario":        func(v *DetectiveCheckDefinitionPin) { v.Scenario.StepID = "../bad" },
		"duplicate-check": func(v *DetectiveCheckDefinitionPin) { v.Dependencies = append(v.Dependencies, v.Check) },
		"missing-check":   func(v *DetectiveCheckDefinitionPin) { v.Dependencies = v.Dependencies[:1] },
		"extra-check": func(v *DetectiveCheckDefinitionPin) {
			c := v.Check
			c.ID = "extra"
			v.Dependencies = append(v.Dependencies, c)
		},
		"dependency-version": func(v *DetectiveCheckDefinitionPin) { v.Dependencies[0].Version = 0 },
		"duplicate-validator": func(v *DetectiveCheckDefinitionPin) {
			v.PublishedValidators = append(v.PublishedValidators, v.PublishedValidators[0])
		},
		"missing-validator": func(v *DetectiveCheckDefinitionPin) { v.PublishedValidators = v.PublishedValidators[:1] },
		"extra-validator": func(v *DetectiveCheckDefinitionPin) {
			x := v.PublishedValidators[0]
			x.ID = "extra"
			v.PublishedValidators = append(v.PublishedValidators, x)
		},
		"duplicate-local": func(v *DetectiveCheckDefinitionPin) {
			v.QueryValidators = append(v.QueryValidators, v.QueryValidators[0])
		},
		"extra-local": func(v *DetectiveCheckDefinitionPin) {
			x := v.QueryValidators[0]
			x.QueryID = "extra"
			v.QueryValidators = append(v.QueryValidators, x)
		},
		"duplicate-query": func(v *DetectiveCheckDefinitionPin) { v.Queries = append(v.Queries, v.Queries[0]) },
		"missing-query":   func(v *DetectiveCheckDefinitionPin) { v.Queries = v.Queries[:1] },
		"extra-query": func(v *DetectiveCheckDefinitionPin) {
			x := v.Queries[0]
			x.QueryID = "extra"
			x.DefinitionJSON = json.RawMessage(`{"id":"extra","title":"Extra","type":"SQL"}`)
			v.Queries = append(v.Queries, x)
		},
		"query-id":        func(v *DetectiveCheckDefinitionPin) { v.Queries[0].QueryID = "../bad" },
		"revision":        func(v *DetectiveCheckDefinitionPin) { v.Queries[0].Revision = " " },
		"revision-budget": func(v *DetectiveCheckDefinitionPin) { v.Queries[0].Revision = strings.Repeat("r", 257) },
		"json":            func(v *DetectiveCheckDefinitionPin) { v.Queries[0].DefinitionJSON = json.RawMessage(`{`) },
		"unknown": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"SQL","providerSecret":"private"}`)
		},
		"duplicate-key": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","id":"z","folderPath":"folder","title":"Z","type":"SQL"}`)
		},
		"trailing": func(v *DetectiveCheckDefinitionPin) { v.Queries[0].DefinitionJSON = json.RawMessage(`{} {}`) },
		"mismatched-id": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"other","folderPath":"folder","title":"Z","type":"SQL"}`)
		},
		"mismatched-folder": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"other","title":"Z","type":"SQL"}`)
		},
		"folder": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"folder"}`)
		},
		"unsupported-spelling": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"TugQL"}`)
		},
		"secret": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"SQL","text":"postgres://alice:private_password@host/db"}`)
		},
		"targets": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"SQL","targets":[{}]}`)
		},
		"query-budget": func(v *DetectiveCheckDefinitionPin) {
			v.Queries[0].DefinitionJSON = json.RawMessage(`{"id":"z","folderPath":"folder","title":"Z","type":"SQL","text":"` + strings.Repeat("a", MaxDetectiveQueryBytes) + `"}`)
		},
		"catalog-budget": func(v *DetectiveCheckDefinitionPin) {
			v.Check.Description = strings.Repeat("a", MaxDetectiveDefinitionBytes)
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			d := definitionFixture()
			change(&d)
			if _, _, err := CanonicalDetectiveDefinition(context.Background(), d); !errors.Is(err, ErrInvalid) || strings.Contains(err.Error(), "private_password") {
				t.Fatalf("unsafe definition accepted: %v", err)
			}
		})
	}
}

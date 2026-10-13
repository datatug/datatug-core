package datatug

import (
	"encoding/json"
	"strings"
	"testing"
)

const semanticFixture = `{"version":1,"basis":{"queryHash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","resultIndex":0,"columns":[{"name":"name","dataType":"text"}]},"meaning":{"format":"meaning/draft-2","id":"query","name":"Query","description":"Query meaning","models":{"query":"query.modelspec.json"},"concepts":[{"id":"person","kind":"entity","labels":{"en":"Person"},"description":"A person"},{"id":"person-name","kind":"property","labels":{"en":"Name"},"description":"Name of a person","of":"person","bindings":[{"model":"modelspec:///query.QueryResult","field":"column1","role":"value"}]}]},"model":{"modelspec":"1.0-draft-2","module":{"id":"datatug/query","name":"query","version":"0.1.0"},"records":{"QueryResult":{"fields":{"column1":{"type":"string"}}}}},"bindings":[{"columnIndex":0,"field":"column1","concept":"person-name"}]}`

func validSemantic(t *testing.T) QuerySemanticModel {
	t.Helper()
	var v QuerySemanticModel
	if err := json.Unmarshal([]byte(semanticFixture), &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestQuerySemanticModelValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*QuerySemanticModel)
	}{
		{"oversized description", func(v *QuerySemanticModel) { v.Meaning.Description = strings.Repeat("x", 4097) }},
		{"oversized bundle", func(v *QuerySemanticModel) {
			for i := 0; i < 100; i++ {
				c := v.Meaning.Concepts[0]
				c.ID = "entity-" + strings.Repeat("a", i+1)
				c.Description = strings.Repeat("x", 4096)
				v.Meaning.Concepts = append(v.Meaning.Concepts, c)
			}
		}},
		{"missing complete field", func(v *QuerySemanticModel) { delete(v.Model.Records.QueryResult.Fields, "column1") }},
		{"wrong column field", func(v *QuerySemanticModel) {
			f := v.Model.Records.QueryResult.Fields["column1"]
			delete(v.Model.Records.QueryResult.Fields, "column1")
			v.Model.Records.QueryResult.Fields["name"] = f
		}},
		{"orphan meaning binding", func(v *QuerySemanticModel) { v.Bindings = []QuerySemanticBinding{} }},
		{"hash", func(v *QuerySemanticModel) { v.Basis.QueryHash = "bad" }},
		{"negative result", func(v *QuerySemanticModel) { v.Basis.ResultIndex = -1 }},
		{"duplicate concept", func(v *QuerySemanticModel) { v.Meaning.Concepts = append(v.Meaning.Concepts, v.Meaning.Concepts[0]) }},
		{"dangling owner", func(v *QuerySemanticModel) { v.Meaning.Concepts[1].Of = "absent" }},
		{"owner property", func(v *QuerySemanticModel) { v.Meaning.Concepts[1].Of = "person-name" }},
		{"entity assertion", func(v *QuerySemanticModel) { v.Meaning.Concepts[0].Bindings = v.Meaning.Concepts[1].Bindings }},
		{"dangling field", func(v *QuerySemanticModel) { v.Bindings[0].Field = "absent" }},
		{"dangling concept", func(v *QuerySemanticModel) { v.Bindings[0].Concept = "absent" }},
		{"missing meaning binding", func(v *QuerySemanticModel) { v.Meaning.Concepts[1].Bindings = nil }},
		{"duplicate binding", func(v *QuerySemanticModel) { v.Bindings = append(v.Bindings, v.Bindings[0]) }},
		{"unsupported type", func(v *QuerySemanticModel) {
			v.Model.Records.QueryResult.Fields["column1"] = QueryModelField{Type: "varchar"}
		}},
		{"credential description", func(v *QuerySemanticModel) { v.Meaning.Description = "password=actual-secret" }},
		{"credential column", func(v *QuerySemanticModel) { v.Basis.Columns[0].Name = "password=actual-secret" }},
		{"credential role", func(v *QuerySemanticModel) { v.Bindings[0].Role = "password=actual-secret" }},
	}
	v := validSemantic(t)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := validSemantic(t)
			tt.mutate(&v)
			if err := v.Validate(); err == nil {
				t.Fatal("accepted invalid bundle")
			}
		})
	}
	// Removing the current column mapping retains reusable definitions.
	v.Bindings = []QuerySemanticBinding{}
	v.Meaning.Concepts[1].Bindings = nil
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var restored QuerySemanticModel
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if len(restored.Meaning.Concepts) != 2 || len(restored.Model.Records.QueryResult.Fields) != 1 {
		t.Fatal("lost definitions")
	}
}
func TestQuerySemanticModelRejectsUnknownFields(t *testing.T) {
	for _, data := range []string{strings.Replace(semanticFixture, `"columnIndex":0,`, ``, 1), strings.Replace(semanticFixture, `"resultIndex":0`, `"resultIndex":null`, 1), strings.Replace(semanticFixture, `"version":1`, `"version":1,"samples":["secret"]`, 1), strings.Replace(semanticFixture, `"type":"string"`, `"type":"string","required":true`, 1)} {
		var v QuerySemanticModel
		if err := json.Unmarshal([]byte(data), &v); err == nil {
			t.Fatal("accepted unsupported metadata")
		}
	}
}

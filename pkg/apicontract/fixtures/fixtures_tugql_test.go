package fixtures

import (
	"encoding/json"
	"testing"

	"github.com/dal-go/dalgo/tugql"
)

// TestFixtures_TugQLFieldsParse proves every fixture's "tugql" field, wherever
// one appears - at the top level (ExecutionRequest) or nested inside another
// object (CaptureQueryRequest.query.tugql) - is real TugQL, a document
// tugql.Deserialize accepts, not just YAML-shaped prose. A frozen fixture is
// an executable example both server (datatug-cli) and client
// (datatug-apps) pin by digest (Manifest); one that merely looks like TugQL
// without being parseable silently teaches every consumer the wrong shape.
//
// This test lives in package fixtures rather than in pkg/apicontract itself
// so that tugql's dependency chain (github.com/dal-go/dalgo/tugql, which pulls
// in gopkg.in/yaml.v3 and the dal query model) never becomes part of what a
// normal importer of pkg/apicontract compiles. pkg/apicontract is the shared
// wire-contract type library; only this _test-exercised package pays for
// tugql. (Test files of a package are never compiled into another package's
// import of it, so even placing this directly in pkg/apicontract would not
// leak the dependency to consumers - but keeping it in fixtures, which
// already imports apicontract for the roundtrip tests, keeps the boundary
// explicit and the intent easy to find.)
func TestFixtures_TugQLFieldsParse(t *testing.T) {
	entries, err := Manifest()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, e := range entries {
		data, err := Read(e.Name)
		if err != nil {
			t.Fatalf("%s: %v", e.Name, err)
		}
		var doc any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Errorf("%s: not JSON: %v", e.Name, err)
			continue
		}
		for _, raw := range tugqlFields(doc) {
			text, ok := raw.(string)
			if !ok {
				t.Errorf("%s: \"tugql\" is not a JSON string: %v", e.Name, raw)
				continue
			}
			if text == "" {
				continue
			}
			if _, err := tugql.Deserialize([]byte(text)); err != nil {
				t.Errorf("%s: \"tugql\" field does not parse as TugQL: %v\ngot:\n%s", e.Name, err, text)
				continue
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no fixture carried a non-empty \"tugql\" field - this test would silently stop proving anything; " +
			"update it (or remove it) if the fixture set no longer demonstrates TugQL")
	}
}

// tugqlFields returns the value of every "tugql" key in doc, at any depth.
func tugqlFields(doc any) []any {
	var found []any
	switch v := doc.(type) {
	case map[string]any:
		for key, value := range v {
			if key == "tugql" {
				found = append(found, value)
				continue
			}
			found = append(found, tugqlFields(value)...)
		}
	case []any:
		for _, item := range v {
			found = append(found, tugqlFields(item)...)
		}
	}
	return found
}

func TestTugQLFields_FindsNestedValues(t *testing.T) {
	var doc any
	if err := json.Unmarshal([]byte(`{"tugql":"a","query":{"tugql":"b"},"list":[{"tugql":"c"}],"n":1}`), &doc); err != nil {
		t.Fatal(err)
	}
	got := map[any]bool{}
	for _, v := range tugqlFields(doc) {
		got[v] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !got[want] {
			t.Errorf("tugqlFields missed %q, got %v", want, got)
		}
	}
}

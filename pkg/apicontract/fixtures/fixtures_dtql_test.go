package fixtures

import (
	"encoding/json"
	"testing"

	"github.com/dal-go/dalgo/dtql"
)

// TestFixtures_DTQLFieldsParse proves every fixture's "dtql" field, wherever
// one appears, is real DTQL - a document dtql.Deserialize accepts - not just
// YAML-shaped prose. A frozen fixture is an executable example both server
// (datatug-cli) and client (datatug-apps) pin by digest (Manifest); one that
// merely looks like DTQL without being parseable silently teaches every
// consumer the wrong shape.
//
// This test lives in package fixtures rather than in pkg/apicontract itself
// so that dtql's dependency chain (github.com/dal-go/dalgo/dtql, which pulls
// in gopkg.in/yaml.v3 and the dal query model) never becomes part of what a
// normal importer of pkg/apicontract compiles. pkg/apicontract is the shared
// wire-contract type library; only this _test-exercised package pays for
// dtql. (Test files of a package are never compiled into another package's
// import of it, so even placing this directly in pkg/apicontract would not
// leak the dependency to consumers - but keeping it in fixtures, which
// already imports apicontract for the roundtrip tests, keeps the boundary
// explicit and the intent easy to find.)
func TestFixtures_DTQLFieldsParse(t *testing.T) {
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
		var generic map[string]json.RawMessage
		if err := json.Unmarshal(data, &generic); err != nil {
			continue // not a flat JSON object fixture - nothing to check here
		}
		raw, ok := generic["dtql"]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err != nil {
			t.Errorf("%s: \"dtql\" is not a JSON string: %v", e.Name, err)
			continue
		}
		if text == "" {
			continue
		}
		if _, err := dtql.Deserialize([]byte(text)); err != nil {
			t.Errorf("%s: \"dtql\" field does not parse as DTQL: %v\ngot:\n%s", e.Name, err, text)
			continue
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no fixture carried a non-empty \"dtql\" field - this test would silently stop proving anything; " +
			"update it (or remove it) if the fixture set no longer demonstrates ad-hoc DTQL")
	}
}

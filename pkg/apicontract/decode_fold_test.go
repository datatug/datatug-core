package apicontract

import (
	"strings"
	"testing"
)

// encoding/json matches a key to a struct field case-insensitively and keeps
// the last of two spellings, so "project" and "Project" in one object are
// duplicates DecodeStrict must reject - but only where a struct field is
// the target: a map keeps both keys.
func TestDecodeStrict_RejectsCaseVariantDuplicateKeys(t *testing.T) {
	const query = `"query":{"folderPath":"","id":"q","title":"t","purpose":"p","source":"s","dtql":"from: X\n","parameters":[],"bindingOrigins":[]}`
	rejected := map[string]string{
		"top level": `{"project":"a","Project":"b","environment":"e","securityContextId":"s","ifNoneMatch":true,` + query + `}`,
		"nested":    `{"project":"a","environment":"e","securityContextId":"s","ifNoneMatch":true,"query":{"folderPath":"","id":"q","ID":"r","title":"t","purpose":"p","source":"s","dtql":"from: X\n","parameters":[],"bindingOrigins":[]}}`,
		"in an array element": `{"project":"a","environment":"e","securityContextId":"s","ifNoneMatch":true,"query":{"folderPath":"","id":"q","title":"t","purpose":"p","source":"s","dtql":"from: X\n",` +
			`"parameters":[{"id":"P","ID":"Q","type":"string","isRequired":true}],"bindingOrigins":[]}}`,
	}
	for name, body := range rejected {
		t.Run(name, func(t *testing.T) {
			var r CaptureQueryRequest
			err := DecodeStrict([]byte(body), &r, "principal", "role")
			if err == nil || !strings.Contains(err.Error(), "duplicate key") {
				t.Fatalf("expected a duplicate-key error, got %v (Project=%q)", err, r.Project)
			}
		})
	}
}

type foldTarget struct {
	Kind   string         `json:"kind"`
	Name   string         `json:"name"`
	Params map[string]int `json:"params"`
	Any    any            `json:"any"`
	Items  []foldItem     `json:"items"`
	foldEmbedded
}

type foldItem struct {
	A int `json:"a"`
}

type foldEmbedded struct {
	Inner string `json:"inner"`
}

func TestDecodeStrict_CaseVariantKeysOnlyWhereAFieldIsTheTarget(t *testing.T) {
	accepted := []string{
		`{"params":{"id":1,"ID":2}}`,
		`{"any":{"k":1,"K":2}}`,
		`{"name":"x","inner":"y","items":[{"a":1},{"A":2}]}`,
	}
	for _, body := range accepted {
		var v foldTarget
		if err := DecodeStrict([]byte(body), &v); err != nil {
			t.Errorf("DecodeStrict(%s) = %v, want nil", body, err)
		}
	}
	rejected := []string{
		"{\"kind\":\"x\",\"\u212aind\":\"y\"}", // the Kelvin sign folds with "k"
		`{"name":"x","NAME":"y"}`,
		`{"inner":"x","Inner":"y"}`,
		`{"items":[{"a":1,"A":2}]}`,
		`{"params":{"id":1,"id":2}}`,
	}
	for _, body := range rejected {
		var v foldTarget
		if err := DecodeStrict([]byte(body), &v); err == nil || !strings.Contains(err.Error(), "duplicate key") {
			t.Errorf("DecodeStrict(%s) = %v, want a duplicate-key error", body, err)
		}
	}
}

func TestFoldJSONName(t *testing.T) {
	for _, pair := range [][2]string{{"project", "PROJECT"}, {"ifMatch", "IFMATCH"}, {"k", "K"}, {"ſ", "S"}} {
		if foldJSONName(pair[0]) != foldJSONName(pair[1]) {
			t.Errorf("foldJSONName(%q) != foldJSONName(%q)", pair[0], pair[1])
		}
	}
	if foldJSONName("a") == foldJSONName("b") {
		t.Error("different names must not fold together")
	}
}

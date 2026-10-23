package meaninggraph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func replaceRootAST(t *testing.T, f *fixture, c *Config, mutate func(map[string]any)) {
	t.Helper()
	v := f.receipt(t, f.rootID)
	updateArchivedJSON(t, f, v, "json", mutate)
	sourcePath := v["source_paths"].(map[string]any)["json"]
	var original map[string]any
	for _, item := range v["files"].([]any) {
		e := item.(map[string]any)
		if e["path"] == sourcePath {
			raw, err := os.ReadFile(filepath.Join(f.root, e["artifactPath"].(string)))
			require.NoError(t, err)
			original, err = parseJSON(raw)
			require.NoError(t, err)
		}
	}
	normalized, err := normalizeAST(original)
	require.NoError(t, err)
	definition := getObject(t, f.root, "definitions/"+f.rootID+".json")
	definition["ast"] = normalized
	raw := putJSON(t, f.root, "definitions/"+f.rootID+".json", definition)
	v["definition"].(map[string]any)["sha256"] = digest(raw)
	v["format_identifier"] = original["modelspec"]
	f.writeReceipt(t, f.rootID, v)
	f.catalog["models"].([]any)[0].(map[string]any)["definition"] = v["definition"]
	sealCatalog(t, f, c)
}
func rootDeclaration(a map[string]any) map[string]any {
	return a["records"].(map[string]any)["Root"].(map[string]any)
}
func TestResolverExactReferenceClosureFailures(t *testing.T) {
	cases := map[string]func(map[string]any){
		"malformed multi-part reference": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"record": "A.B.C"}
		},
		"unknown module requires exact lock": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"record": "Foreign.Record"}
		},
		"missing own declaration": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"record": "A.Absent"}
		},
		"missing enum group": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"enum": "Status"}
		},
		"use requires array":           func(a map[string]any) { rootDeclaration(a)["use"] = "Part" },
		"use requires string":          func(a map[string]any) { rootDeclaration(a)["use"] = []any{true} },
		"use requires known component": func(a map[string]any) { rootDeclaration(a)["use"] = []any{"Absent"} },
		"record requires string": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"record": true}
		},
		"component reference missing": func(a map[string]any) {
			rootDeclaration(a)["fields"].(map[string]any)["bad"] = map[string]any{"component": "Absent"}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := newFixture(t)
			replaceRootAST(t, f, &c, mutate)
			r, e := NewResolver(c)
			require.NoError(t, e)
			_, e = r.Resolve(context.Background(), f.ref)
			require.Error(t, e)
		})
	}
	for _, bound := range []bool{false, true} {
		t.Run(map[bool]string{false: "conflicting revision alias", true: "closure model bound"}[bound], func(t *testing.T) {
			f, c := newFixture(t)
			v := f.receipt(t, f.rootID)
			v["dependency_pins"] = append(v["dependency_pins"].([]any), map[string]any{"alias": "A", "revision_id": f.oldID})
			f.writeReceipt(t, f.rootID, v)
			if bound {
				c.Limits.MaxModels = 2
			}
			r, e := NewResolver(c)
			require.NoError(t, e)
			_, e = r.Resolve(context.Background(), f.ref)
			if bound {
				require.ErrorContains(t, e, "closure bound")
			} else {
				require.ErrorContains(t, e, "conflicting exact revisions")
			}
		})
	}
}
func TestResolverEarlierVocabularyAndLiteralEnums(t *testing.T) {
	f, c := newFixture(t)
	replaceRootAST(t, f, &c, func(a map[string]any) {
		a["modelspec"] = "1.0-draft"
		records := a["records"].(map[string]any)
		delete(a, "records")
		a["entities"] = records
		for _, value := range records {
			d := value.(map[string]any)
			d["properties"] = d["fields"]
			delete(d, "fields")
			for _, v := range d["properties"].(map[string]any) {
				field := v.(map[string]any)
				if target, ok := field["record"]; ok {
					delete(field, "record")
					field["entity"] = target
				}
			}
		}
		fields := records["Root"].(map[string]any)["properties"].(map[string]any)
		fields["local"] = map[string]any{"entity": "A.Root"}
		fields["status"] = map[string]any{"enum": []any{"yes", "no"}}
	})
	r, err := NewResolver(c)
	require.NoError(t, err)
	got, err := r.Resolve(context.Background(), f.ref)
	require.NoError(t, err)
	var source map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(got.Root.SourceJSON, &source))
	require.Contains(t, source, "entities")
	require.NotContains(t, source, "records")
}

func TestResolverModelWithoutComponentsAndComponentOnlySource(t *testing.T) {
	for _, componentOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "model only", true: "component only"}[componentOnly], func(t *testing.T) {
			f, c := newFixture(t)
			v := f.receipt(t, f.rootID)
			v["dependency_pins"] = []any{}
			f.writeReceipt(t, f.rootID, v)
			replaceRootAST(t, f, &c, func(a map[string]any) {
				if componentOnly {
					delete(a, "records")
				} else {
					delete(a, "components")
					d := rootDeclaration(a)
					delete(d, "use")
					delete(d["fields"].(map[string]any), "child")
				}
			})
			r, err := NewResolver(c)
			require.NoError(t, err)
			result, err := r.Resolve(context.Background(), f.ref)
			require.NoError(t, err)
			require.Len(t, result.Revisions, 1)
		})
	}
}

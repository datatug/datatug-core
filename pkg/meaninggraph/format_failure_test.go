package meaninggraph

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func sealCatalog(t *testing.T, f *fixture, c *Config) {
	t.Helper()
	delete(f.catalog, "checksum")
	body, err := canonical(f.catalog)
	require.NoError(t, err)
	f.catalog["checksum"] = "sha256:" + digest(body)
	raw := putJSON(t, f.root, "catalog.json", f.catalog)
	c.CatalogSHA256 = digest(raw)
}
func replaceIndex(t *testing.T, f *fixture, c *Config, body []byte) {
	t.Helper()
	putBytes(t, f.root, "revisions/index.json", body)
	f.catalog["revision_index"].(map[string]any)["sha256"] = digest(body)
	sealCatalog(t, f, c)
}
func TestResolverRejectsMalformedNativeCatalogAndIndex(t *testing.T) {
	type change func(*testing.T, *fixture, *Config)
	catalogCase := func(fn func(map[string]any)) change {
		return func(t *testing.T, f *fixture, c *Config) { fn(f.catalog); sealCatalog(t, f, c) }
	}
	indexCase := func(fn func(map[string]any)) change {
		return func(t *testing.T, f *fixture, c *Config) {
			v := getObject(t, f.root, "revisions/index.json")
			fn(v)
			raw, err := json.Marshal(v)
			require.NoError(t, err)
			replaceIndex(t, f, c, raw)
		}
	}
	cases := map[string]change{
		"invalid catalog JSON": func(t *testing.T, f *fixture, c *Config) {
			raw := []byte(`{"broken":`)
			putBytes(t, f.root, "catalog.json", raw)
			c.CatalogSHA256 = digest(raw)
		},
		"unsupported catalog": catalogCase(func(v map[string]any) { v["format"] = "future" }),
		"catalog internal checksum": func(t *testing.T, f *fixture, c *Config) {
			f.catalog["checksum"] = "sha256:" + strings.Repeat("0", 64)
			raw := putJSON(t, f.root, "catalog.json", f.catalog)
			c.CatalogSHA256 = digest(raw)
		},
		"index descriptor object": catalogCase(func(v map[string]any) { v["revision_index"] = nil }),
		"index path":              catalogCase(func(v map[string]any) { v["revision_index"].(map[string]any)["path"] = "other.json" }),
		"index hash":              catalogCase(func(v map[string]any) { v["revision_index"].(map[string]any)["sha256"] = "bad" }),
		"index JSON":              func(t *testing.T, f *fixture, c *Config) { replaceIndex(t, f, c, []byte(`{"broken":`)) },
		"index format":            indexCase(func(v map[string]any) { v["format"] = "future" }),
		"index rows array":        indexCase(func(v map[string]any) { v["revisions"] = nil }),
		"index bound":             indexCase(func(v map[string]any) { v["revisions"] = make([]any, 10001) }),
		"index row object":        indexCase(func(v map[string]any) { v["revisions"].([]any)[0] = nil }),
		"index row ID":            indexCase(func(v map[string]any) { v["revisions"].([]any)[0].(map[string]any)["id"] = "bad" }),
		"index row address": indexCase(func(v map[string]any) {
			v["revisions"].([]any)[0].(map[string]any)["address"] = "modelspec://elsewhere/o/r/m"
		}),
		"index row source commit": indexCase(func(v map[string]any) { v["revisions"].([]any)[0].(map[string]any)["commit"] = "main" }),
		"index row duplicate":     indexCase(func(v map[string]any) { rows := v["revisions"].([]any); v["revisions"] = append(rows, rows[0]) }),
		"source pins object":      catalogCase(func(v map[string]any) { v["source_revisions"] = nil }),
		"selections array":        catalogCase(func(v map[string]any) { v["models"] = nil }),
		"selection object":        catalogCase(func(v map[string]any) { v["models"].([]any)[0] = nil }),
		"selection address":       catalogCase(func(v map[string]any) { v["models"].([]any)[0].(map[string]any)["address"] = "invalid" }),
		"selection duplicate":     catalogCase(func(v map[string]any) { rows := v["models"].([]any); v["models"] = append(rows, rows[0]) }),
		"selected pin mismatch": catalogCase(func(v map[string]any) {
			v["source_revisions"].(map[string]any)["https://github.com/example/a"] = strings.Repeat("0", 40)
		}),
		"selected pin missing":       catalogCase(func(v map[string]any) { v["source_revisions"] = map[string]any{} }),
		"selected definition object": catalogCase(func(v map[string]any) { v["models"].([]any)[0].(map[string]any)["definition"] = nil }),
		"selected definition path": catalogCase(func(v map[string]any) {
			v["models"].([]any)[0].(map[string]any)["definition"].(map[string]any)["path"] = "../escape"
		}),
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := newFixture(t)
			mutate(t, f, &c)
			r, e := NewResolver(c)
			require.NoError(t, e)
			_, e = r.Resolve(context.Background(), f.ref)
			require.Error(t, e)
		})
	}
}

func updateArchivedJSON(t *testing.T, f *fixture, receipt map[string]any, which string, mutate func(map[string]any)) {
	t.Helper()
	files := receipt["files"].([]any)
	path := receipt["source_paths"].(map[string]any)[which]
	for _, v := range files {
		entry := v.(map[string]any)
		if entry["path"] != path {
			continue
		}
		data, err := os.ReadFile(filepath.Join(f.root, entry["artifactPath"].(string)))
		require.NoError(t, err)
		value, err := parseJSON(data)
		require.NoError(t, err)
		mutate(value)
		body, err := json.Marshal(value)
		require.NoError(t, err)
		a := archive(t, f.root, body)
		entry["sha256"] = a["sha256"]
		entry["artifactPath"] = a["artifactPath"]
		return
	}
	t.Fatal("source not found")
}
func TestResolverRejectsMalformedReceiptsAndArchivedIdentity(t *testing.T) {
	cases := map[string]func(*testing.T, *fixture, map[string]any){
		"receipt address":     func(t *testing.T, f *fixture, v map[string]any) { v["address"] = "invalid" },
		"registration object": func(t *testing.T, f *fixture, v map[string]any) { v["source_record"] = nil },
		"registration provenance": func(t *testing.T, f *fixture, v map[string]any) {
			v["source_record"].(map[string]any)["repository"] = "https://arbitrary.invalid/registry"
		},
		"registration path": func(t *testing.T, f *fixture, v map[string]any) {
			v["source_record"].(map[string]any)["path"] = "models/$records/other.yaml"
		},
		"acquisition evidence": func(t *testing.T, f *fixture, v map[string]any) { v["acquisition_record"] = nil },
		"files empty":          func(t *testing.T, f *fixture, v map[string]any) { v["files"] = []any{} },
		"file object":          func(t *testing.T, f *fixture, v map[string]any) { v["files"].([]any)[0] = nil },
		"file repeated path": func(t *testing.T, f *fixture, v map[string]any) {
			a := v["files"].([]any)
			v["files"] = append(a, a[0])
		},
		"source paths object": func(t *testing.T, f *fixture, v map[string]any) { v["source_paths"] = nil },
		"source pair missing": func(t *testing.T, f *fixture, v map[string]any) {
			v["source_paths"].(map[string]any)["json"] = "absent.json"
		},
		"module object":             func(t *testing.T, f *fixture, v map[string]any) { v["module"] = nil },
		"licences object":           func(t *testing.T, f *fixture, v map[string]any) { v["licences"] = nil },
		"maintainers empty":         func(t *testing.T, f *fixture, v map[string]any) { v["maintainers"] = []any{} },
		"maintainer string":         func(t *testing.T, f *fixture, v map[string]any) { v["maintainers"] = []any{true} },
		"logical descriptor object": func(t *testing.T, f *fixture, v map[string]any) { v["logical_digest"] = nil },
		"logical digest algorithm": func(t *testing.T, f *fixture, v map[string]any) {
			v["logical_digest"].(map[string]any)["algorithm"] = "future"
		},
		"definition object": func(t *testing.T, f *fixture, v map[string]any) { v["definition"] = nil },
		"definition unsafe path": func(t *testing.T, f *fixture, v map[string]any) {
			v["definition"].(map[string]any)["path"] = "../definition"
		},
		"dependency array":  func(t *testing.T, f *fixture, v map[string]any) { v["dependency_pins"] = nil },
		"dependency object": func(t *testing.T, f *fixture, v map[string]any) { v["dependency_pins"] = []any{nil} },
		"unsupported archived source": func(t *testing.T, f *fixture, v map[string]any) {
			updateArchivedJSON(t, f, v, "json", func(a map[string]any) { a["modelspec"] = "future" })
			v["format_identifier"] = "future"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := newFixture(t)
			v := f.receipt(t, f.rootID)
			mutate(t, f, v)
			f.writeReceipt(t, f.rootID, v)
			r, e := NewResolver(c)
			require.NoError(t, e)
			_, e = r.Resolve(context.Background(), f.ref)
			require.Error(t, e)
		})
	}
	for _, missing := range []string{"receipt", "registration", "source"} {
		t.Run("missing "+missing, func(t *testing.T) {
			f, c := newFixture(t)
			v := f.receipt(t, f.rootID)
			path := "revisions/$records/" + f.rootID + ".yaml"
			if missing == "registration" {
				path = v["source_record"].(map[string]any)["artifactPath"].(string)
			}
			if missing == "source" {
				path = v["files"].([]any)[0].(map[string]any)["artifactPath"].(string)
			}
			require.NoError(t, os.Remove(filepath.Join(f.root, path)))
			r, e := NewResolver(c)
			require.NoError(t, e)
			_, e = r.Resolve(context.Background(), f.ref)
			require.Error(t, e)
		})
	}
}

func TestArchivedRegistrationYAMLShapeAndAliases(t *testing.T) {
	valid := []byte("format: modelspec-registry/draft-1\nmodule: &name A\ntitle: *name\nmaintainers: &people [fixture]\nignored: true\n")
	got, err := parseRegistration(valid)
	require.NoError(t, err)
	require.Equal(t, "A", got["title"])
	require.Equal(t, []any{"fixture"}, got["maintainers"])
	got, err = parseRegistration([]byte("names: &people [fixture]\nmaintainers: *people\n"))
	require.NoError(t, err)
	require.Equal(t, []any{"fixture"}, got["maintainers"])
	for _, raw := range []string{"[", "[]", "", "module: A\nmodule: B\n", "1: A\n", "module: 2\n", "maintainers: fixture\n", "maintainers: [1]\n", "maintainers: &self [*self]\n", "a: &x hello\nb: &y *x\n"} {
		_, err = parseRegistration([]byte(raw))
		require.Error(t, err, raw)
	}
}

func replaceArchiveEvidence(t *testing.T, f *fixture, entry map[string]any, body []byte) {
	t.Helper()
	a := archive(t, f.root, body)
	entry["sha256"] = a["sha256"]
	entry["artifactPath"] = a["artifactPath"]
}
func TestResolverRawRetainedDocumentFailures(t *testing.T) {
	cases := map[string]func(*testing.T, *fixture, *Config){
		"receipt JSON": func(t *testing.T, f *fixture, c *Config) {
			putBytes(t, f.root, "revisions/$records/"+f.rootID+".yaml", []byte(`{"broken":`))
		},
		"registration JSON": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			replaceArchiveEvidence(t, f, v["source_record"].(map[string]any), []byte(`{"broken":`))
			f.writeReceipt(t, f.rootID, v)
		},
		"registration address": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			e := v["source_record"].(map[string]any)
			reg := getObject(t, f.root, e["artifactPath"].(string))
			reg["address"] = "invalid"
			body, err := json.Marshal(reg)
			require.NoError(t, err)
			replaceArchiveEvidence(t, f, e, body)
			f.writeReceipt(t, f.rootID, v)
		},
		"source JSON": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			replaceArchiveEvidence(t, f, v["files"].([]any)[1].(map[string]any), []byte(`{"broken":`))
			f.writeReceipt(t, f.rootID, v)
		},
		"definition JSON": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			body := []byte(`{"broken":`)
			putBytes(t, f.root, "definitions/"+f.rootID+".json", body)
			v["definition"].(map[string]any)["sha256"] = digest(body)
			f.writeReceipt(t, f.rootID, v)
		},
		"module numeric complexity": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			n := json.Number("1e" + strings.Repeat("9", 129))
			v["module"].(map[string]any)["extra"] = n
			updateArchivedJSON(t, f, v, "json", func(a map[string]any) { a["module"].(map[string]any)["extra"] = n })
			f.writeReceipt(t, f.rootID, v)
		},
		"AST numeric complexity": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			updateArchivedJSON(t, f, v, "json", func(a map[string]any) {
				rootDeclaration(a)["fields"].(map[string]any)["id"].(map[string]any)["max_len"] = json.Number("1e" + strings.Repeat("9", 129))
			})
			f.writeReceipt(t, f.rootID, v)
		},
		"selection numeric complexity": func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			v["definition"].(map[string]any)["extra"] = json.Number("1e" + strings.Repeat("9", 129))
			f.writeReceipt(t, f.rootID, v)
			f.catalog["models"].([]any)[0].(map[string]any)["definition"] = v["definition"]
			sealCatalog(t, f, c)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			f, c := newFixture(t)
			mutate(t, f, &c)
			r, err := NewResolver(c)
			require.NoError(t, err)
			_, err = r.Resolve(context.Background(), f.ref)
			require.Error(t, err)
		})
	}
}

func TestResolverRegistrationRepositoryCaseSemantics(t *testing.T) {
	for _, tc := range []struct {
		name, repository string
		accepted         bool
	}{
		{"ASCII case", "HTTPS://GITHUB.COM/EXAMPLE/A", true},
		{"long s must not fold to ASCII s", "httpſ://github.com/example/a", false},
		{"different publisher", "https://github.com/example/other", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newFixture(t)
			v := f.receipt(t, f.rootID)
			entry := v["source_record"].(map[string]any)
			registration := getObject(t, f.root, entry["artifactPath"].(string))
			registration["repository"] = tc.repository
			body, err := json.Marshal(registration)
			require.NoError(t, err)
			replaceArchiveEvidence(t, f, entry, body)
			f.writeReceipt(t, f.rootID, v)
			r, err := NewResolver(c)
			require.NoError(t, err)
			_, err = r.Resolve(context.Background(), f.ref)
			if tc.accepted {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "source/module/attribution identity mismatch")
			}
		})
	}
}

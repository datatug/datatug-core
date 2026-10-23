package meaninggraph

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	root                            string
	catalog                         map[string]any
	rows                            []any
	ref, history                    datatug.DataModelReference
	rootID, depID, oldID, meaningID string
}

func putJSON(t *testing.T, root, path string, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	require.NoError(t, err)
	putBytes(t, root, path, body)
	return body
}
func putBytes(t *testing.T, root, path string, body []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0700))
	require.NoError(t, os.WriteFile(full, body, 0600))
}
func getObject(t *testing.T, root, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	require.NoError(t, err)
	v, err := parseJSON(raw)
	require.NoError(t, err)
	return v
}
func archive(t *testing.T, root string, body []byte) map[string]any {
	hash := digest(body)
	path := artifactPath(hash)
	putBytes(t, root, path, body)
	return map[string]any{"sha256": hash, "artifactPath": path}
}
func (f *fixture) receipt(t *testing.T, id string) map[string]any {
	return getObject(t, f.root, "revisions/$records/"+id+".yaml")
}
func (f *fixture) writeReceipt(t *testing.T, id string, value any) {
	putJSON(t, f.root, "revisions/$records/"+id+".yaml", value)
}
func (f *fixture) flush(t *testing.T) Config {
	idx := putJSON(t, f.root, "revisions/index.json", map[string]any{"format": "meaninggraph-revision-index/draft-1", "revisions": f.rows})
	f.catalog["revision_index"] = map[string]any{"path": "revisions/index.json", "sha256": digest(idx)}
	delete(f.catalog, "checksum")
	canonicalBody, err := canonical(f.catalog)
	require.NoError(t, err)
	f.catalog["checksum"] = "sha256:" + digest(canonicalBody)
	raw := putJSON(t, f.root, "catalog.json", f.catalog)
	return Config{RegistryCommit: strings.Repeat("f", 40), CatalogSHA256: digest(raw), FixtureRoot: f.root}
}
func modelFixture(t *testing.T, root, alias, commit string, records, components map[string]any, pins []any) (revisionRow, map[string]any) {
	address := "modelspec://github.com/example/" + strings.ToLower(alias) + "/" + alias
	repo := "https://github.com/example/" + strings.ToLower(alias)
	id := revisionID("model", address, commit)
	module := map[string]any{"id": "github.com/example/" + strings.ToLower(alias) + "/model/" + alias, "name": alias, "version": "1"}
	ast := map[string]any{"modelspec": "1.0-draft-2", "module": module, "records": records, "components": components}
	raw, err := json.Marshal(ast)
	require.NoError(t, err)
	// The compact fixture retains a matching HCL/JSON model pair, including
	// the exact large numeric token. Source interpretation is not executed.
	var source strings.Builder
	source.WriteString("# SPDX-License-Identifier: MIT\n")
	for _, group := range []struct{ key, kind string }{{"components", "component"}, {"records", "record"}} {
		declarations := ast[group.key].(map[string]any)
		names := make([]string, 0, len(declarations))
		for name := range declarations {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			declaration := declarations[name].(map[string]any)
			source.WriteString(group.kind + " " + strconv.Quote(name) + " {\n")
			for _, attr := range []string{"use", "key"} {
				if value, ok := declaration[attr]; ok {
					raw, _ := json.Marshal(value)
					source.WriteString("  " + attr + " = " + string(raw) + "\n")
				}
			}
			fieldMap := declaration["fields"].(map[string]any)
			fieldNames := make([]string, 0, len(fieldMap))
			for field := range fieldMap {
				fieldNames = append(fieldNames, field)
			}
			sort.Strings(fieldNames)
			for _, field := range fieldNames {
				source.WriteString("  field " + strconv.Quote(field) + " {\n")
				attrs := fieldMap[field].(map[string]any)
				keys := make([]string, 0, len(attrs))
				for key := range attrs {
					keys = append(keys, key)
				}
				sort.Strings(keys)
				for _, key := range keys {
					raw, _ := json.Marshal(attrs[key])
					source.WriteString("    " + key + " = " + string(raw) + "\n")
				}
				source.WriteString("  }\n")
			}
			source.WriteString("}\n")
		}
	}
	hcl := []byte(source.String())
	files := []any{}
	for _, source := range []struct {
		path, media string
		body        []byte
	}{{"model/" + alias + ".modelspec.hcl", "application/hcl", hcl}, {"model/" + alias + ".modelspec.json", "application/json", raw}, {"LICENSE", "text/plain", []byte("MIT\n")}} {
		entry := archive(t, root, source.body)
		entry["path"] = source.path
		entry["mediaType"] = source.media
		files = append(files, entry)
	}
	registration := map[string]any{"format": "modelspec-registry/draft-1", "module": alias, "address": address, "repository": repo, "commit": commit, "title": "Fixture " + alias, "source_file": "model/" + alias + ".modelspec.hcl", "json_file": "model/" + alias + ".modelspec.json", "licence": "MIT", "maintainers": []any{"fixture"}}
	registrationRaw, err := json.Marshal(registration)
	require.NoError(t, err)
	evidence := archive(t, root, registrationRaw)
	evidence["repository"] = LegacyRegistryRepository
	evidence["commit"] = strings.Repeat("e", 40)
	evidence["path"] = "models/$records/" + strings.ToLower(alias) + ".yaml"
	def := map[string]any{"format": "meaninggraph-definition/draft-1", "kind": "model", "revision_id": id, "ast": ast}
	definition := putJSON(t, root, "definitions/"+id+".json", def)
	definitionRef := map[string]any{"path": "definitions/" + id + ".json", "sha256": digest(definition)}
	if pins == nil {
		pins = []any{}
	}
	receipt := map[string]any{"format": "meaninggraph-revision/draft-1", "id": id, "kind": "model", "address": address, "registry_id": strings.ToLower(alias), "repository": repo, "commit": commit, "source_record": evidence, "format_identifier": "1.0-draft-2", "module": module, "source_paths": map[string]any{"hcl": "model/" + alias + ".modelspec.hcl", "json": "model/" + alias + ".modelspec.json"}, "dependency_pins": pins, "licences": map[string]any{"model": "MIT"}, "maintainers": []any{"fixture"}, "logical_digest": map[string]any{"algorithm": "meaninggraph-logical-digest/v1", "sha256": strings.Repeat("1", 64)}, "files": files, "definition": definitionRef}
	putJSON(t, root, "revisions/$records/"+id+".yaml", receipt)
	selected := map[string]any{"id": strings.ToLower(alias), "address": address, "repository": repo, "commit": commit, "revision_id": id, "module_id": module["id"], "module_version": "1", "title": "Fixture " + alias, "definition": definitionRef}
	return revisionRow{id, "model", address, commit, "revisions/$records/" + id + ".yaml"}, selected
}
func newFixture(t *testing.T) (*fixture, Config) {
	root := t.TempDir()
	ca, cb, old := strings.Repeat("a", 40), strings.Repeat("b", 40), strings.Repeat("c", 40)
	ida := revisionID("model", "modelspec://github.com/example/a/A", ca)
	idb := revisionID("model", "modelspec://github.com/example/b/B", cb)
	part := map[string]any{"Part": map[string]any{"fields": map[string]any{"self": map[string]any{"component": "Part"}}}}
	a, selection := modelFixture(t, root, "A", ca, map[string]any{"Root": map[string]any{"key": []any{"id"}, "use": []any{"Part"}, "fields": map[string]any{"id": map[string]any{"type": "string", "max_len": json.Number("900719925474099312345678901")}, "child": map[string]any{"record": "B.Node"}}}}, part, []any{map[string]any{"alias": "B", "revision_id": idb}})
	b, _ := modelFixture(t, root, "B", cb, map[string]any{"Node": map[string]any{"fields": map[string]any{"parent": map[string]any{"record": "A.Root"}}}}, map[string]any{}, []any{map[string]any{"alias": "A", "revision_id": ida}})
	history, _ := modelFixture(t, root, "A", old, map[string]any{"OldRoot": map[string]any{"fields": map[string]any{"old": map[string]any{"type": "string"}}}}, map[string]any{}, nil)
	f := &fixture{root: root, ref: datatug.DataModelReference{Address: a.Address, Commit: ca}, history: datatug.DataModelReference{Address: a.Address, Commit: old}, rootID: a.ID, depID: b.ID, oldID: history.ID}
	for _, r := range []revisionRow{a, b, history} {
		f.rows = append(f.rows, map[string]any{"id": r.ID, "kind": r.Kind, "address": r.Address, "commit": r.Commit, "path": r.Path})
	}
	f.meaningID = revisionID("meaning", "meaning://github.com/example/semantics", strings.Repeat("d", 40))
	f.rows = append(f.rows, map[string]any{"id": f.meaningID, "kind": "meaning", "address": "meaning://github.com/example/semantics", "commit": strings.Repeat("d", 40), "path": "revisions/$records/" + f.meaningID + ".yaml"})
	f.catalog = map[string]any{"format": "meaninggraph-catalog/draft-1", "models": []any{selection}, "graphs": []any{}, "source_revisions": map[string]any{"https://github.com/example/a": ca}}
	return f, f.flush(t)
}
func TestResolverCurrentHistoryCyclesRawNumbersAndNoMeanings(t *testing.T) {
	f, c := newFixture(t)
	r, err := NewResolver(c)
	require.NoError(t, err)
	got, err := r.Resolve(context.Background(), f.ref)
	require.NoError(t, err)
	require.Equal(t, TransportFixture, got.Transport)
	require.Len(t, got.Revisions, 2)
	require.Contains(t, string(got.Root.SourceJSON), "900719925474099312345678901")
	require.Contains(t, string(got.Root.Definition), "900719925474099312345678901")
	for _, path := range got.ReadPaths {
		require.NotContains(t, path, f.meaningID, "a meaning receipt must never be fetched")
	}
	for _, file := range got.Root.Sources {
		body, err := os.ReadFile(filepath.Join(f.root, artifactPath(file.SHA256)))
		require.NoError(t, err)
		require.Equal(t, body, file.Bytes)
	}
	originalReceipt, err := os.ReadFile(filepath.Join(f.root, "revisions/$records/"+f.rootID+".yaml"))
	require.NoError(t, err)
	require.Equal(t, originalReceipt, []byte(got.Root.Receipt))
	old, err := r.Resolve(context.Background(), f.history)
	require.NoError(t, err)
	require.Equal(t, f.oldID, old.Root.RevisionID)
	require.Len(t, old.Revisions, 1)
	require.Contains(t, string(old.Root.SourceJSON), "OldRoot")
	upper := f.ref
	upper.Address = "modelspec://github.com/Example/A/A"
	same, err := r.Resolve(context.Background(), upper)
	require.NoError(t, err)
	require.Equal(t, upper, same.Reference)
	require.Equal(t, f.rootID, same.Root.RevisionID)
}
func TestResolverRejectsMissingPinsHashesAndIdentityWithoutLatestFallback(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*testing.T, *fixture, *Config)
	}{
		{"catalog digest", func(t *testing.T, f *fixture, c *Config) { c.CatalogSHA256 = strings.Repeat("0", 64) }},
		{"missing history", func(t *testing.T, f *fixture, c *Config) {
			f.ref = f.history
			f.rows = append(f.rows[:2], f.rows[3:]...)
			*c = f.flush(t)
		}},
		{"receipt identity", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			v["commit"] = strings.Repeat("0", 40)
			f.writeReceipt(t, f.rootID, v)
		}},
		{"source digest", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			files := v["files"].([]any)
			path := files[0].(map[string]any)["artifactPath"].(string)
			putBytes(t, f.root, path, []byte("tampered"))
		}},
		{"definition digest", func(t *testing.T, f *fixture, c *Config) {
			putBytes(t, f.root, "definitions/"+f.rootID+".json", []byte(`{}`))
		}},
		{"unsafe artifact", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			v["files"].([]any)[0].(map[string]any)["artifactPath"] = "../outside"
			f.writeReceipt(t, f.rootID, v)
		}},
		{"missing dependency", func(t *testing.T, f *fixture, c *Config) { f.rows = append(f.rows[:1], f.rows[2:]...); *c = f.flush(t) }},
		{"semantic dependency", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			v["dependency_pins"].([]any)[0].(map[string]any)["revision_id"] = f.meaningID
			f.writeReceipt(t, f.rootID, v)
		}},
		{"alias mismatch", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			v["dependency_pins"].([]any)[0].(map[string]any)["alias"] = "Wrong"
			f.writeReceipt(t, f.rootID, v)
		}},
		{"ambiguous locks", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			pins := v["dependency_pins"].([]any)
			v["dependency_pins"] = append(pins, pins[0])
			f.writeReceipt(t, f.rootID, v)
		}},
		{"wrong source registration", func(t *testing.T, f *fixture, c *Config) {
			v := f.receipt(t, f.rootID)
			ref := v["source_record"].(map[string]any)
			raw, err := os.ReadFile(filepath.Join(f.root, ref["artifactPath"].(string)))
			require.NoError(t, err)
			reg, err := parseJSON(raw)
			require.NoError(t, err)
			reg["commit"] = strings.Repeat("0", 40)
			body, err := json.Marshal(reg)
			require.NoError(t, err)
			evidence := archive(t, f.root, body)
			ref["sha256"] = evidence["sha256"]
			ref["artifactPath"] = evidence["artifactPath"]
			f.writeReceipt(t, f.rootID, v)
		}},
		{"selected definition disagrees", func(t *testing.T, f *fixture, c *Config) {
			f.catalog["models"].([]any)[0].(map[string]any)["definition"].(map[string]any)["sha256"] = strings.Repeat("0", 64)
			*c = f.flush(t)
		}},
		{"index digest", func(t *testing.T, f *fixture, c *Config) { putBytes(t, f.root, "revisions/index.json", []byte(`{}`)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, c := newFixture(t)
			tc.mutate(t, f, &c)
			r, err := NewResolver(c)
			require.NoError(t, err)
			_, err = r.Resolve(context.Background(), f.ref)
			require.Error(t, err)
		})
	}
}
func TestResolverDefinitionMatchesOriginalJSONWithoutFloatRounding(t *testing.T) {
	f, c := newFixture(t)
	receipt := f.receipt(t, f.rootID)
	def := getObject(t, f.root, "definitions/"+f.rootID+".json")
	field := def["ast"].(map[string]any)["records"].(map[string]any)["Root"].(map[string]any)["fields"].(map[string]any)["id"].(map[string]any)
	field["max_len"] = json.Number("900719925474099312345678902")
	raw := putJSON(t, f.root, "definitions/"+f.rootID+".json", def)
	receipt["definition"].(map[string]any)["sha256"] = digest(raw)
	f.writeReceipt(t, f.rootID, receipt)
	f.catalog["models"].([]any)[0].(map[string]any)["definition"] = receipt["definition"]
	c = f.flush(t)
	r, err := NewResolver(c)
	require.NoError(t, err)
	_, err = r.Resolve(context.Background(), f.ref)
	require.ErrorContains(t, err, "definition differs")
}
func TestResolverExplicitConfigurationBoundsAndFixtureIsolation(t *testing.T) {
	_, err := NewResolver(Config{})
	require.Error(t, err)
	f, c := newFixture(t)
	for _, limit := range []Limits{{MaxFiles: 1}, {MaxModels: 1}, {MaxFileBytes: 1}, {MaxTotalBytes: 1}} {
		config := c
		config.Limits = limit
		r, e := NewResolver(config)
		require.NoError(t, e)
		_, e = r.Resolve(context.Background(), f.ref)
		require.Error(t, e)
	}
	r, err := NewResolver(c)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = r.Resolve(ctx, f.ref)
	require.ErrorIs(t, err, context.Canceled)
	require.NoError(t, os.Remove(filepath.Join(f.root, "catalog.json")))
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err, "fixture mode must never fall back to network")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestResolverFixedGitHubTransportAndRedirectRefusal(t *testing.T) {
	f, c := newFixture(t)
	fixtureRoot := c.FixtureRoot
	c.FixtureRoot = ""
	r, err := NewResolver(c)
	require.NoError(t, err)
	calls := []string{}
	r.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https", req.URL.Scheme)
		require.Equal(t, "raw.githubusercontent.com", req.URL.Host)
		require.Nil(t, req.URL.User)
		require.Empty(t, req.Header.Get("Authorization"))
		require.Empty(t, req.Header.Get("Cookie"))
		prefix := "/meaninggraph/registry/" + c.RegistryCommit + "/"
		require.True(t, strings.HasPrefix(req.URL.Path, prefix))
		path := strings.TrimPrefix(req.URL.Path, prefix)
		calls = append(calls, path)
		body, e := os.ReadFile(filepath.Join(fixtureRoot, filepath.FromSlash(path)))
		require.NoError(t, e)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header), Request: req}, nil
	})
	result, err := r.Resolve(context.Background(), f.ref)
	require.NoError(t, err)
	require.Equal(t, TransportGitHub, result.Transport)
	require.NotEmpty(t, calls)
	redirects := 0
	r.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		redirects++
		return &http.Response{StatusCode: 302, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{"Location": []string{"https://arbitrary.invalid/secret"}}, Request: req}, nil
	})
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err)
	require.Equal(t, 1, redirects)
	r.client.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		<-req.Context().Done()
		return nil, req.Context().Err()
	})
	r.config.Limits.Timeout = time.Millisecond
	_, err = r.Resolve(context.Background(), f.ref)
	require.Error(t, err)
}
func TestCanonicalJSONAndExactNumericComparison(t *testing.T) {
	value := map[string]any{"html": "<>&\u2028", "z": json.Number("900719925474099312345678901"), "𐀀": json.Number("1"), "": json.Number("2")}
	got, err := canonical(value)
	require.NoError(t, err)
	require.Equal(t, "{\"html\":\"<>&\u2028\",\"z\":900719925474099312345678901,\"𐀀\":1,\"\":2}", string(got))
	same, err := equal(json.Number("1000.00"), json.Number("1e3"))
	require.NoError(t, err)
	require.True(t, same)
	same, err = equal(json.Number("9007199254740993123"), json.Number("9007199254740993124"))
	require.NoError(t, err)
	require.False(t, same)
	large, err := parseJSON([]byte(`{"number":1e1000,"nested":[{"x":2}]}`))
	require.NoError(t, err)
	require.Equal(t, json.Number("1e1000"), large["number"])
	same, err = equal(large["number"], json.Number("10e999"))
	require.NoError(t, err)
	require.True(t, same)
	for _, raw := range []string{`{"a":1}{"b":2}`, `{"a":{"b":1,"b":2}}`, `{"a":[{"b":1,"b":2}]}`} {
		_, err := parseJSON([]byte(raw))
		require.Error(t, err)
	}
	_, err = decimal("1e" + strings.Repeat("9", 129))
	require.Error(t, err)
	_, err = parseJSON([]byte(`{"id":"a","id":"b"}`))
	require.Error(t, err)
	_, err = parseJSON([]byte(`{"x":"\ud800"}`))
	require.Error(t, err)
}

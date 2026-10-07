package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// This is the v1 shape used by datatug-demo-project: source and descriptor
// remain provenance, never executable resolver output.
const fixtureConnections = `{
  "format": "datatug-demo-connections/v1",
  "source": {"registry": "https://example.test/registry?password=secret"},
  "connections": [
    {"id":"chinook-sqlite","dataset":"chinook","storage":"sqlite","tags":["chinook","sqlite"],"environments":["dev"],"readiness":"public-api","source":"https://example.test/ovdb?password=secret","descriptor":"https://example.test/descriptor?token=secret","query":"ovdb-read"},
    {"id":"chinook-postgresql","dataset":"chinook","storage":"postgresql","tags":["chinook","postgresql"],"environments":["QA"],"readiness":"hosted-api-pending","source":"https://example.test/chinook?password=secret","query":"setup-required"},
    {"id":"chinook-ingitdb","dataset":"chinook","storage":"ingitdb","tags":["chinook","ingitdb"],"environments":["dev"],"readiness":"hosted-repository","source":"https://example.test/tree?password=secret","manifest":"https://example.test/manifest?token=secret","query":"local-checkout-required"}
  ],
  "bigQueryEditions": [
    {"id":"chinook-bigquery","dataset":"chinook","storage":"bigquery","tags":["chinook","bigquery"],"sourceProjectId":"demodb-dev","datasetId":"chinook","location":"US","readiness":"public-read-user-project-required","verification":"https://example.test/verify?token=secret","query":"not-enabled-in-browser"}
  ],
  "bigQueryPlans": [{"id":"chinook-pending-bigquery","readiness":"setup-required"}]
}`

func testConnectionProject(t *testing.T) (string, datatug.ProjectConnectionsReader, datatug.EnvironmentConnectionResolver) {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "connections"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "connections", "demo-db.json"), []byte(fixtureConnections), 0600); err != nil {
		t.Fatal(err)
	}
	for env, ids := range map[string][]string{
		"dev": {"chinook-sqlite", "chinook-ingitdb"},
		"QA":  {"chinook-postgresql"},
		"BQ":  {"chinook-bigquery", "chinook-pending-bigquery"},
	} {
		folder := filepath.Join(root, "environments", env)
		if err := os.MkdirAll(folder, 0700); err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(map[string]any{"id": env, "dbServers": []any{}, "editionConnections": ids})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(folder, env+".env.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	store := NewProjectStore("demo", root)
	return root, store.(datatug.ProjectConnectionsReader), store.(datatug.EnvironmentConnectionResolver)
}

func TestEnvironmentConnectionResolver(t *testing.T) {
	_, reader, resolver := testConnectionProject(t)
	ctx := context.Background()
	all, err := reader.LoadProjectConnections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Connections) != 3 || len(all.BigQueryEditions) != 1 || len(all.BigQueryPlans) != 1 {
		t.Fatalf("unexpected catalog shape: %+v", all)
	}
	for _, tc := range []struct{ env, id, storage, readiness string }{
		{"dev", "chinook-sqlite", "sqlite", "public-api"},
		{"dev", "chinook-ingitdb", "ingitdb", "hosted-repository"},
		{"BQ", "chinook-bigquery", "bigquery", "public-read-user-project-required"},
	} {
		got, err := resolver.ResolveEnvironmentConnection(ctx, tc.env, tc.id)
		if err != nil {
			t.Fatalf("%s/%s: %v", tc.env, tc.id, err)
		}
		if got.Storage != tc.storage || got.Readiness != tc.readiness {
			t.Fatalf("%s/%s: %+v", tc.env, tc.id, got)
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), "https://") || strings.Contains(string(data), "secret") {
			t.Fatalf("remote source leaked: %s", data)
		}
	}
	if got, err := resolver.ResolveEnvironmentConnection(ctx, "BQ", "chinook-bigquery"); err != nil || got.SourceProjectID != "demodb-dev" || got.DatasetID != "chinook" {
		t.Fatalf("structured BQ metadata: %+v, %v", got, err)
	}
	if _, err := resolver.ResolveEnvironmentConnection(ctx, "dev", "legacy-only-catalog"); !errors.Is(err, datatug.ErrConnectionNotFound) {
		t.Fatalf("legacy catalog fallback unavailable: %v", err)
	}
	for _, tc := range []struct {
		env, id string
		kind    error
	}{
		{"QA", "chinook-postgresql", datatug.ErrConnectionNotReady},
		{"BQ", "chinook-pending-bigquery", datatug.ErrConnectionNotReady},
		{"QA", "chinook-bigquery", datatug.ErrConnectionNotInEnvironment},
		{"dev", "chinook-postgresql", datatug.ErrConnectionNotInEnvironment},
	} {
		got, err := resolver.ResolveEnvironmentConnection(ctx, tc.env, tc.id)
		if !errors.Is(err, tc.kind) {
			t.Fatalf("%s/%s: got %v, want %v", tc.env, tc.id, err, tc.kind)
		}
		if tc.id == "chinook-postgresql" && tc.env == "QA" && (got.ID != tc.id || got.Storage != "postgresql" || got.Readiness != "hosted-api-pending") {
			t.Fatalf("pending identity metadata missing: %+v, %v", got, err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "https://") {
			t.Fatalf("source leaked in error: %v", err)
		}
	}
	_, err = resolver.ResolveEnvironmentConnection(ctx, "../QA", "chinook-postgresql")
	if err == nil {
		t.Fatal("unsafe environment ID accepted")
	}
}

func TestProjectConnectionsRejectsAmbiguityAndMalformedFiles(t *testing.T) {
	root, reader, resolver := testConnectionProject(t)
	ctx := context.Background()
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "connections", name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("second.json", `{"format":"datatug-demo-connections/v1","connections":[{"id":"chinook-sqlite","dataset":"chinook","storage":"sqlite","readiness":"public-api"}]}`)
	if _, err := reader.LoadProjectConnections(ctx); !errors.Is(err, datatug.ErrAmbiguousConnection) {
		t.Fatalf("duplicate ID: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "connections", "second.json")); err != nil {
		t.Fatal(err)
	}
	write("second.json", `{"format":"future/v2"}`)
	if _, err := reader.LoadProjectConnections(ctx); !errors.Is(err, datatug.ErrUnsupportedConnectionFile) {
		t.Fatalf("format: %v", err)
	}
	write("second.json", `{"format":"datatug-demo-connections/v1"} {}`)
	if _, err := reader.LoadProjectConnections(ctx); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	write("second.json", `{"format":"datatug-demo-connections/v1","connections":[],"Connections":[]}`)
	if _, err := reader.LoadProjectConnections(ctx); err == nil {
		t.Fatal("ambiguous JSON keys accepted")
	}
	if err := os.Remove(filepath.Join(root, "connections", "second.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "connections", "demo-db.json"), filepath.Join(root, "connections", "second.json")); err == nil {
		if _, err := reader.LoadProjectConnections(ctx); err == nil {
			t.Fatal("symlink catalog accepted")
		}
	}
	if err := os.Remove(filepath.Join(root, "connections", "second.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "connections", "demo-db.json"), make([]byte, maxConnectionCatalogSize+1), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.LoadProjectConnections(ctx); err == nil {
		t.Fatal("oversized catalog accepted")
	}
	if err := os.RemoveAll(filepath.Join(root, "connections")); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.LoadProjectConnections(ctx); !os.IsNotExist(err) {
		t.Fatalf("legacy project fallback unavailable: %v", err)
	}
	_, err := resolver.ResolveEnvironmentConnection(ctx, "QA", "chinook-postgresql")
	if !errors.Is(err, datatug.ErrConnectionNotFound) {
		t.Fatalf("legacy resolver fallback unavailable: %v", err)
	}
}

func TestEnvironmentConnectionResolverRejectsConflictingMembership(t *testing.T) {
	root, _, resolver := testConnectionProject(t)
	ctx := context.Background()
	path := filepath.Join(root, "environments", "QA", "QA.env.json")
	if err := os.WriteFile(path, []byte(`{"id":"QA","dbServers":[],"editionConnections":["chinook-sqlite"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveEnvironmentConnection(ctx, "QA", "chinook-sqlite"); !errors.Is(err, datatug.ErrConnectionNotInEnvironment) {
		t.Fatalf("declaration mismatch: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"QA","dbServers":[],"editionConnections":["chinook-sqlite","chinook-sqlite"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveEnvironmentConnection(ctx, "QA", "chinook-sqlite"); !errors.Is(err, datatug.ErrAmbiguousConnection) {
		t.Fatalf("duplicate env IDs: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"id":"QA","dbServers":[],"editionConnections":["chinook-postgresql"],"EditionConnections":["chinook-sqlite"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveEnvironmentConnection(ctx, "QA", "chinook-postgresql"); err == nil {
		t.Fatal("ambiguous environment JSON keys accepted")
	}
}

// Pinned from datatug/datatug-demo-project origin/main 51716f3 on 2026-10-07.
// The real published v1 manifest and environment files exercise all 18
// editions, including fields unknown to the safe metadata model.
func TestPublishedDemoConnectionManifestCompatibility(t *testing.T) {
	root := t.TempDir()
	copyFixture := func(from, to string) {
		t.Helper()
		data, err := os.ReadFile(filepath.Join("testdata", from))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, to)), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, to), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	copyFixture("demo-db-v1.json", "connections/demo-db.json")
	copyFixture("demo-dev-environment.json", "environments/dev/dev.env.json")
	copyFixture("demo-QA-environment.json", "environments/QA/QA.env.json")
	store := NewProjectStore("demo", root)
	reader := store.(datatug.ProjectConnectionsReader)
	resolver := store.(datatug.EnvironmentConnectionResolver)
	catalog, err := reader.LoadProjectConnections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Connections) != 18 || len(catalog.BigQueryEditions) != 6 || len(catalog.BigQueryPlans) != 2 {
		t.Fatalf("published fixture changed: %d/%d/%d", len(catalog.Connections), len(catalog.BigQueryEditions), len(catalog.BigQueryPlans))
	}
	for _, tc := range []struct {
		env, id, storage string
		pending          bool
	}{
		{"dev", "chinook-sqlite", "sqlite", false},
		{"dev", "chinook-ingitdb", "ingitdb", false},
		{"QA", "chinook-postgresql", "postgresql", true},
	} {
		got, err := resolver.ResolveEnvironmentConnection(context.Background(), tc.env, tc.id)
		if errors.Is(err, datatug.ErrConnectionNotReady) != tc.pending || (!tc.pending && err != nil) {
			t.Fatalf("%s/%s: %v", tc.env, tc.id, err)
		}
		if got.ID != tc.id || got.Storage != tc.storage {
			t.Fatalf("%s/%s: %+v", tc.env, tc.id, got)
		}
	}
	if _, err := resolver.ResolveEnvironmentConnection(context.Background(), "QA", "chinook-bigquery"); !errors.Is(err, datatug.ErrConnectionNotInEnvironment) {
		t.Fatalf("BigQuery edition with no environment membership resolved: %v", err)
	}
}

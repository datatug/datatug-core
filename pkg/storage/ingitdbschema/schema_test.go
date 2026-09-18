package ingitdbschema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingitdb/ingitdb-go/ingitdb"
	"github.com/ingitdb/ingitdb-go/ingitdb/validator"
	"gopkg.in/yaml.v3"
)

const queriesDefinitionPath = "files/ext/.collection/subcollections/projects/subcollections/queries/definition.yaml"

// recordFileShape mirrors the record_file mapping's expected keys, used to
// assert the AC without depending on ingitdb.CollectionDef's own (looser)
// yaml tags.
type recordFileShape struct {
	Name       string `yaml:"name"`
	Format     string `yaml:"format"`
	Type       string `yaml:"type"`
	RecordsDir string `yaml:"records_dir"`
}

// definitionShape decodes a definition.yaml loosely enough to check both
// that record_file carries the four keys and that records_dir does NOT also
// appear as a sibling of record_file at the top level.
type definitionShape struct {
	RecordFile recordFileShape `yaml:"record_file"`
	// RecordsDir catches a sibling `records_dir:` key, if present. It MUST be
	// nil for a correctly authored definition.
	RecordsDir *string `yaml:"records_dir"`
}

// TestQueriesDefinitionDeclaresCanonicalRecordFile verifies
// dalgo-project-store#ac:collection-declares-the-canonical-record-file: the
// authored `queries` definition.yaml (inherited suffix `query`) declares a
// record_file mapping carrying name/format/type and a nested records_dir,
// with records_dir written inside record_file and not as a sibling key.
func TestQueriesDefinitionDeclaresCanonicalRecordFile(t *testing.T) {
	content, err := schemaFS.ReadFile(queriesDefinitionPath)
	if err != nil {
		t.Fatalf("read embedded %s: %v", queriesDefinitionPath, err)
	}

	var def definitionShape
	if err = yaml.Unmarshal(content, &def); err != nil {
		t.Fatalf("unmarshal %s: %v", queriesDefinitionPath, err)
	}

	if got, want := def.RecordFile.Name, "{key}/{key}.query.json"; got != want {
		t.Errorf("record_file.name = %q, want %q", got, want)
	}
	if got, want := def.RecordFile.Format, "json"; got != want {
		t.Errorf("record_file.format = %q, want %q", got, want)
	}
	if got, want := def.RecordFile.Type, "map[string]any"; got != want {
		t.Errorf("record_file.type = %q, want %q", got, want)
	}
	if got, want := def.RecordFile.RecordsDir, "."; got != want {
		t.Errorf("record_file.records_dir = %q, want %q", got, want)
	}
	if def.RecordsDir != nil {
		t.Errorf("records_dir must be nested inside record_file, found a sibling top-level records_dir=%q", *def.RecordsDir)
	}
}

// TestSchemaLoadsThroughStrictReader loads every collection and
// subcollection this package ships through ingitdb-go v0.6.1's strict
// reader (validator.ReadDefinition with ingitdb.Validate()), the same
// reader dalgo2ingitdb uses to open a store. A copy written by WriteSchema
// into a fresh directory MUST load without error.
func TestSchemaLoadsThroughStrictReader(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}

	def, err := validator.ReadDefinition(dir, ingitdb.Validate())
	if err != nil {
		t.Fatalf("ReadDefinition(%s): %v", dir, err)
	}

	ext, ok := def.Collections["ext"]
	if !ok {
		t.Fatalf("root collection %q not found; got %v", "ext", collectionIDs(def))
	}
	projects, ok := ext.SubCollections["projects"]
	if !ok {
		t.Fatalf("subcollection %q not found under %q", "projects", "ext")
	}
	for _, leaf := range []string{
		"queries", "entities", "environments", "dbmodels", "boards",
		"recordsets", "folders", "dbdrivers",
	} {
		if _, ok = projects.SubCollections[leaf]; !ok {
			t.Errorf("subcollection %q not found under %q", leaf, "projects")
		}
	}
	// credentials is deliberately not declared: secrets-vault will add it
	// later with real fields (founder decision, 2026-09-17).
	if _, ok = projects.SubCollections["credentials"]; ok {
		t.Errorf("subcollection %q should not be declared yet", "credentials")
	}
	environments := projects.SubCollections["environments"]
	if environments != nil {
		for _, leaf := range []string{"servers", "catalogs"} {
			if _, ok = environments.SubCollections[leaf]; !ok {
				t.Errorf("subcollection %q not found under %q", leaf, "projects/environments")
			}
		}
	}
	dbdrivers := projects.SubCollections["dbdrivers"]
	if dbdrivers != nil {
		if _, ok = dbdrivers.SubCollections["dbservers"]; !ok {
			t.Errorf("subcollection %q not found under %q", "dbservers", "projects/dbdrivers")
		}
	}
}

func collectionIDs(def *ingitdb.Definition) []string {
	ids := make([]string, 0, len(def.Collections))
	for id := range def.Collections {
		ids = append(ids, id)
	}
	return ids
}

// TestSiblingRecordsDirFailsToLoad proves the mechanism behind the AC: a
// definition carrying `records_dir` as a sibling of `record_file` (rather
// than nested inside it) is rejected by the strict KnownFields(true) reader
// as an unknown field, not silently accepted.
func TestSiblingRecordsDirFailsToLoad(t *testing.T) {
	dir := t.TempDir()
	if err := WriteSchema(dir); err != nil {
		t.Fatalf("WriteSchema(%s): %v", dir, err)
	}

	brokenDefinition := []byte(`record_file:
    name: '{key}/{key}.query.json'
    format: json
    type: map[string]any
records_dir: '.'
columns:
    id:
        type: string
        required: true
`)
	target := filepath.Join(dir, filepath.FromSlash(
		"ext/.collection/subcollections/projects/subcollections/queries/definition.yaml"))
	if err := os.WriteFile(target, brokenDefinition, 0o644); err != nil {
		t.Fatalf("write broken definition: %v", err)
	}

	_, err := validator.ReadDefinition(dir, ingitdb.Validate())
	if err == nil {
		t.Fatal("ReadDefinition succeeded over a definition with a sibling records_dir; want an unknown-field error")
	}
	// Pin down the actual mechanism, not merely that some error occurred: the
	// strict KnownFields(true) yaml decoder must reject records_dir as a field
	// unknown to ingitdb.CollectionDef (it only exists nested inside
	// record_file), not fail for an unrelated reason such as a missing file.
	const wantSubstring = "field records_dir not found in type ingitdb.CollectionDef"
	if !strings.Contains(err.Error(), wantSubstring) {
		t.Fatalf("error = %q, want it to contain %q", err.Error(), wantSubstring)
	}
}

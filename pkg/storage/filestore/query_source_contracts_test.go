package filestore

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

// This is the complete, unfiltered demo-project-1 tree from Demo commit
// 9277f638803d6b1fcf4b1380bd5b727747605288, exported with git archive.
func sourceContractDemo(t *testing.T) string {
	t.Helper()
	file, err := os.Open("testdata/demo-project-1-9277f63.tar.gz")
	require.NoError(t, err)
	defer func() { require.NoError(t, file.Close()) }()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	defer func() { require.NoError(t, gz.Close()) }()
	root := t.TempDir()
	reader := tar.NewReader(gz)
	files := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		require.True(t, filepath.IsLocal(header.Name))
		path := filepath.Join(root, header.Name)
		switch header.Typeflag {
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeDir:
			require.NoError(t, os.MkdirAll(path, 0700))
		case tar.TypeReg:
			require.NoError(t, os.MkdirAll(filepath.Dir(path), 0700))
			data, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(path, data, 0600))
			files++
		default:
			t.Fatalf("unexpected archive entry %s", header.Name)
		}
	}
	require.Equal(t, 750, files, "complete source fixture must remain unfiltered")
	return filepath.Join(root, "demo-project-1")
}

func TestExistingSourceMetadataFullDemoAdmission(t *testing.T) {
	path := sourceContractDemo(t)
	project, err := NewProjectStore("demo-project-1", path).LoadProject(context.Background())
	require.NoError(t, err)
	require.NoError(t, project.Validate())
}

func TestExistingSourceMetadataActualQueriesRevisionedPersistence(t *testing.T) {
	path := sourceContractDemo(t)
	ctx := context.Background()
	original := NewProjectStore("demo-project-1", path).(datatug.RevisionedQueriesStore)
	for _, id := range []string{"demodb/chinook-billing-server", "demodb/chinook-customer-country-counts-server", "demodb/chinook-customer-invoice-join", "demodb/chinook-invoice-compare", "demodb/customer-country-comparison-server", "demodb/northwind-customer-country-counts-server", "hosted/chinook-customer-preview"} {
		t.Run(id, func(t *testing.T) {
			loaded, err := original.LoadQueryRevision(ctx, id)
			require.NoError(t, err)
			metadata, err := os.ReadFile(filepath.Join(path, "queries", id+".query.json"))
			require.NoError(t, err)
			var expected datatug.QueryDef
			require.NoError(t, json.Unmarshal(metadata, &expected))
			body := readDemoQueryBody(t, filepath.Join(path, "queries", id))
			expected.Text = string(body)
			require.Equal(t, expected.Federation, loaded.Query.Federation)
			require.Equal(t, expected.RelationshipBindings, loaded.Query.RelationshipBindings)
			require.Equal(t, expected.Text, loaded.Query.Text)
			require.Equal(t, expected.Purpose, loaded.Query.Purpose)
			require.Equal(t, expected.Parameters, loaded.Query.Parameters)
			require.Equal(t, expected.Recordsets, loaded.Query.Recordsets)
			require.True(t, loaded.Query.Federation != nil && loaded.Query.Federation.ExpectedServerIdentity != nil || len(loaded.Query.RelationshipBindings) > 0)
			store := NewProjectStore("copy", t.TempDir()).(datatug.RevisionedQueriesStore)
			q := loaded.Query
			first, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
			require.NoError(t, err)
			again, err := store.LoadQueryRevision(ctx, id)
			require.NoError(t, err)
			require.Equal(t, first, again)
			// An actual identity edit changes the revision; stale CAS cannot overwrite it.
			if q.Federation != nil && q.Federation.ExpectedServerIdentity != nil {
				q.Federation.ExpectedServerIdentity.ServerID += ":next"
			} else {
				q.RelationshipBindings[0].Version += ":next"
			}
			second, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
			require.NoError(t, err)
			require.NotEqual(t, first.Revision, second.Revision)
			require.Equal(t, first.Query.CreatedAt, second.Query.CreatedAt)
			_, err = store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: first.Revision})
			require.Error(t, err)
			if len(q.RelationshipBindings) > 0 {
				q.RelationshipBindings[0].Version += ":next"
				third, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: second.Revision})
				require.NoError(t, err)
				require.NotEqual(t, second.Revision, third.Revision)
			}
		})
	}
}

func TestExistingSourceMetadataFullDemoRejectsMalformedDefinition(t *testing.T) {
	for _, mode := range []string{"null", "credential", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			path := sourceContractDemo(t)
			file := filepath.Join(path, "queries/demodb/chinook-billing-server.query.json")
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			var q map[string]any
			require.NoError(t, json.Unmarshal(data, &q))
			federation := q["federation"].(map[string]any)
			switch mode {
			case "null":
				federation["expectedServerIdentity"] = nil
			case "credential":
				federation["expectedServerIdentity"].(map[string]any)["serverId"] = "Password=secret"
			case "unknown":
				federation["expectedServerIdentity"].(map[string]any)["grant"] = true
			}
			changed, err := json.Marshal(q)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(file, changed, 0600))
			project, err := NewProjectStore("demo-project-1", path).LoadProject(context.Background())
			if err == nil {
				require.Error(t, project.Validate())
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestExistingSourceMetadataExactDefaultsPersist(t *testing.T) {
	for _, value := range []string{"9223372036854775807", "9007199254740993"} {
		t.Run(value, func(t *testing.T) {
			var q datatug.QueryDefWithFolderPath
			require.NoError(t, json.Unmarshal([]byte(`{"id":"exact","title":"Exact","type":"SQL","text":"SELECT @input","parameters":[{"id":"input","type":"integer","defaultValue":`+value+`}],"relationshipBindings":[{"id":"pin","version":"001","from":{"schema":"","table":"A"},"to":{"schema":"","table":"B"},"pairs":[{"fromField":"id","toField":"id"}]}]}`), &q))
			store := NewProjectStore("exact", t.TempDir()).(datatug.RevisionedQueriesStore)
			saved, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
			require.NoError(t, err)
			loaded, err := store.LoadQueryRevision(context.Background(), "exact")
			require.NoError(t, err)
			require.Equal(t, saved.Revision, loaded.Revision)
			require.Equal(t, json.Number(value), loaded.Query.Parameters[0].DefaultValue)
		})
	}
}

// readDemoQueryBody reads the body of the demo query at base ("<dir>/<id>"):
// the TugQL text or structured form.
func readDemoQueryBody(t *testing.T, base string) []byte {
	t.Helper()
	for _, ext := range []string{"tugql", tugqlStructuredBodyExt} {
		if body, err := os.ReadFile(base + ".query." + ext); err == nil {
			return body
		}
	}
	t.Fatalf("no body file for %s", base)
	return nil
}

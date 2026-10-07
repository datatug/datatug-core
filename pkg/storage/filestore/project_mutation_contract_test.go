package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/stretchr/testify/require"
)

// This exercises the public mutation envelope against the current project
// reader/writer, including the exact two-file layout the Git adapter must
// reproduce. DALgo project-store is a later conformance target.
func TestSaveQueryEnvelopeCurrentLayout(t *testing.T) {
	const wire = `{"storage":"local","project":"demo","operationId":"save-1","ifNoneMatch":true,"query":{"folderPath":"~","id":"customer","title":"Customers","type":"DTQL","text":"SELECT CustomerId FROM Customer","federation":{"ovdbBaseUrl":"https://demodb.dev/ovdb","tables":[{"database":"chinook","name":"Customer","fields":["CustomerId"]}]}}}`
	var request dto.SaveQueryRequest
	require.NoError(t, json.Unmarshal([]byte(wire), &request))
	require.NoError(t, request.Validate())
	projectDir := t.TempDir()
	store := NewProjectStore("demo", projectDir).(datatug.RevisionedQueriesStore)
	ctx := context.Background()
	query := request.Query
	query.FolderPath = "" // the local store represents the shared root with an empty path
	created, err := store.PutQuery(ctx, &query, datatug.QueryWriteCondition{IfNoneMatch: request.IfNoneMatch})
	require.NoError(t, err)
	require.NotEmpty(t, created.Revision)
	metadata, err := os.ReadFile(filepath.Join(projectDir, "queries", "customer.query.json"))
	require.NoError(t, err)
	require.NotContains(t, string(metadata), request.Query.Text)
	require.Contains(t, string(metadata), `"ovdbBaseUrl"`)
	contained, err := os.ReadFile(filepath.Join(projectDir, "queries", "customer.query.dtql"))
	require.NoError(t, err)
	require.Equal(t, request.Query.Text, string(contained))
	loaded, err := store.LoadQueryRevision(ctx, "customer")
	require.NoError(t, err)
	require.Equal(t, request.Query.Federation, loaded.Query.Federation)
	require.Equal(t, request.Query.Text, loaded.Query.Text)
	require.Equal(t, created.Revision, loaded.Revision)

	updated := loaded.Query
	updated.Text = "SELECT FirstName FROM Customer"
	saved, err := store.PutQuery(ctx, &updated, datatug.QueryWriteCondition{IfMatch: loaded.Revision})
	require.NoError(t, err)
	require.NotEqual(t, loaded.Revision, saved.Revision)
	_, err = store.PutQuery(ctx, &query, datatug.QueryWriteCondition{IfMatch: loaded.Revision})
	var conflict *datatug.QueryRevisionConflictError
	require.True(t, errors.As(err, &conflict), "stale writer must fail: %v", err)
	after, err := store.LoadQueryRevision(ctx, "customer")
	require.NoError(t, err)
	require.Equal(t, saved.Revision, after.Revision)
}

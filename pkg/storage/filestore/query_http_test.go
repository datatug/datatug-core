package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func TestHTTPQueryRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewProjectStore("test", dir).(datatug.RevisionedQueriesStore)
	q := datatug.QueryDefWithFolderPath{FolderPath: "api"}
	q.ID, q.Title, q.Type, q.Text = "gh", "GitHub", datatug.QueryTypeHTTP, "https://api.github.com/repos/x"
	q.HTTP = &datatug.QueryHTTPRequest{Method: "POST", Headers: []datatug.HTTPHeaderItem{{Name: "Accept", Value: "application/json"}}, Body: `{"q":1}`}
	_, err := store.PutQuery(context.Background(), &q, datatug.QueryWriteCondition{IfNoneMatch: true})
	require.NoError(t, err)
	loaded, err := store.LoadQueryRevision(context.Background(), "api/gh")
	require.NoError(t, err)
	require.Equal(t, q.HTTP, loaded.Query.HTTP)
	require.Equal(t, q.Text, loaded.Query.Text)

	// The URL stays in the .query.http sidecar; method/headers/body in the JSON definition.
	var sidecar, meta string
	require.NoError(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			switch {
			case strings.HasSuffix(p, "gh.query.http"):
				sidecar = string(b)
			case strings.HasSuffix(p, "gh.query.json"):
				meta = string(b)
			}
		}
		return err
	}))
	require.Equal(t, q.Text, sidecar)
	require.Contains(t, meta, `"http"`)
	require.NotContains(t, meta, "github.com")
}

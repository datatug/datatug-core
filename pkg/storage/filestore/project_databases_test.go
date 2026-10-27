package filestore

import (
	"context"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProjectDatabasesRoundTrip(t *testing.T) {
	ctx := context.Background()
	store := newFsProjectStore("p", t.TempDir())
	p := modelProject()
	p.Databases = datatug.ProjectDatabases{{ID: "music", ConnectionID: "music", OVDB: datatug.PublicOVDBDatabase{BaseURL: "https://public.example", DatabaseID: "music"}}}
	require.NoError(t, store.SaveProject(ctx, p))
	loaded, err := store.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	require.Equal(t, p.Databases, loaded.Databases)
	loaded.Databases = nil
	require.NoError(t, store.SaveProject(ctx, loaded))
	file, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.Databases, file.Databases)
	loaded.Databases = datatug.ProjectDatabases{}
	require.NoError(t, store.SaveProject(ctx, loaded))
	file, err = store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Empty(t, file.Databases)
}

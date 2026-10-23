package filestore

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func TestProjectCoordinationRoundTripAndOldClientSave(t *testing.T) {
	ctx := context.Background()
	store := newFsProjectStore("p", t.TempDir())
	p := modelProject()
	p.Coordination = &datatug.WatchCoordination{Endpoint: "http://127.0.0.1:8988", Namespace: "northwind"}
	require.NoError(t, store.SaveProject(ctx, p))
	loaded, err := store.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	require.Equal(t, p.Coordination, loaded.Coordination)
	legacy := modelProject()
	legacy.Title = "Saved by older client"
	require.NoError(t, store.SaveProject(ctx, legacy))
	file, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.Coordination, file.Coordination)
	p.Coordination.Endpoint = "https://new-coordinator.example.com"
	require.NoError(t, store.SaveProject(ctx, p))
	file, err = store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.Coordination, file.Coordination)
}

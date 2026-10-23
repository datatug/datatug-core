package dalgostore_test

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
)

func TestProjectCoordinationDALGORoundTrip(t *testing.T) {
	ctx := context.Background()
	store := dalgostore.NewProjectStore(newTestDB(), "p")
	p := projectWithAccess(store, "p", "private")
	p.Coordination = &datatug.WatchCoordination{Endpoint: "https://coord.example.com", Namespace: "p"}
	require.NoError(t, store.SaveProject(ctx, p))
	loaded, err := store.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	require.Equal(t, p.Coordination, loaded.Coordination)
	legacy := projectWithAccess(store, "p", "private")
	legacy.Title = "Older client save"
	require.NoError(t, store.SaveProject(ctx, legacy))
	file, err := store.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.Coordination, file.Coordination)
}

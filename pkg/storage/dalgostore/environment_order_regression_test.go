package dalgostore_test

import (
	"context"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestProjectEnvironmentOrderRoundTrip(t *testing.T) {
	ctx := context.Background()
	s := dalgostore.NewProjectStore(newTestDB(), "p")
	p := projectWithAccess(s, "p", "private")
	p.Title = "Project"
	p.EnvironmentOrder = []string{"prod", "dev"}
	require.NoError(t, s.SaveProject(ctx, p))
	loaded, err := s.LoadProject(ctx, datatug.Depth(1))
	require.NoError(t, err)
	require.Equal(t, p.EnvironmentOrder, loaded.EnvironmentOrder)
	require.NoError(t, s.SaveProject(ctx, loaded))
	file, err := s.LoadProjectFile(ctx)
	require.NoError(t, err)
	require.Equal(t, p.EnvironmentOrder, file.EnvironmentOrder)
}

func TestItemEnvironmentOrderUsesCallerMetadata(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "p")
	s, err := dalgostore.NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	for _, id := range []string{"dev", "local", "prod"} {
		require.NoError(t, s.SaveEnvironment(ctx, &datatug.Environment{ProjectItem: datatug.ProjectItem{ID: id}}))
	}
	list, err := s.LoadEnvironments(ctx)
	require.NoError(t, err)
	require.Equal(t, "local", list[0].ID)
	order := []string{"prod", "local", "dev"}
	ordered, err := dalgostore.NewProjectItemsStore(db, "p", root, dalgostore.WithEnvironmentOrder(order))
	require.NoError(t, err)
	order[0] = "dev"
	list, err = ordered.LoadEnvironments(ctx)
	require.NoError(t, err)
	require.Equal(t, "prod", list[0].ID)
	_, err = ordered.LoadProjectFile(ctx)
	require.ErrorIs(t, err, dalgostore.ErrNotImplemented)
}

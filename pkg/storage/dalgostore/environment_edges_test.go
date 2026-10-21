package dalgostore_test

import (
	"context"
	"testing"

	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
)

func TestEnvironmentBatchSummaryAndNativeOrder(t *testing.T) {
	ctx := context.Background()
	s := dalgostore.NewProjectStore(newTestDB(), "p")
	p := projectWithAccess(s, "p", "private")
	p.EnvironmentOrder = []string{"prod", "dev"}
	require.NoError(t, s.SaveProject(ctx, p))
	values := datatug.Environments{{ProjectItem: datatug.ProjectItem{ID: "dev", Title: "Development"}}, {ProjectItem: datatug.ProjectItem{ID: "prod"}}}
	require.NoError(t, s.SaveEnvironments(ctx, values))
	summary, err := s.LoadEnvironmentSummary(ctx, "dev")
	require.NoError(t, err)
	require.Equal(t, "Development", summary.Title)
	list, err := s.LoadEnvironments(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"prod", "dev"}, list.IDs())
	require.ErrorIs(t, s.SaveEnvironment(ctx, nil), dalgostore.ErrAssetInvalid)
	require.ErrorIs(t, s.SaveEnvironments(ctx, datatug.Environments{nil}), dalgostore.ErrAssetInvalid)
	_, err = s.LoadEnvironmentSummary(ctx, "missing")
	require.True(t, record.IsNotFound(err))
}
func TestEnvironmentLoadOrderFailures(t *testing.T) {
	ctx := context.Background()
	s := dalgostore.NewProjectStore(failingDB{DB: newTestDB(), failQuery: true}, "p")
	_, err := s.LoadEnvironments(ctx)
	require.ErrorIs(t, err, errBackend)
	s = dalgostore.NewProjectStore(newTestDB(), "missing")
	_, err = s.LoadEnvironments(ctx)
	require.ErrorIs(t, err, datatug.ErrProjectDoesNotExist)
	root := record.NewKeyWithID("projects", "p")
	s, err = dalgostore.NewProjectItemsStore(newTestDB(), "p", root, dalgostore.WithEnvironmentOrder([]string{string([]byte{255})}))
	require.NoError(t, err)
	_, err = s.LoadEnvironments(ctx)
	require.ErrorContains(t, err, "invalid Unicode")
}

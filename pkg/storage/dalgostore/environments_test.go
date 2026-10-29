package dalgostore_test

import (
	"context"
	"encoding/json"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage/dalgostore"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEnvironmentCallerTransactionAndIsolation(t *testing.T) {
	ctx := context.Background()
	db := newTestDB()
	root := record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "one"), "projects", "p")
	var e datatug.Environment
	require.NoError(t, json.Unmarshal([]byte(`{"id":"dev","title":"Development","presentation":{"tone":"warning"},"dbServers":[],"editionConnections":["music-ovdb"],"private":{"token":"secret"}}`), &e))
	require.NoError(t, db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		s, err := dalgostore.NewProjectItemsStore(tx, "p", root)
		require.NoError(t, err)
		return s.SaveEnvironment(ctx, &e)
	}))
	s, err := dalgostore.NewProjectItemsStore(db, "p", root)
	require.NoError(t, err)
	got, err := s.LoadEnvironment(ctx, "dev")
	require.NoError(t, err)
	body, err := json.Marshal(got)
	require.NoError(t, err)
	require.Contains(t, string(body), `"tone":"warning"`)
	summary, err := s.LoadEnvironmentSummary(ctx, "dev")
	require.NoError(t, err)
	summaryBody, err := json.Marshal(summary)
	require.NoError(t, err)
	var summaryWire datatug.EnvironmentSummary
	require.NoError(t, json.Unmarshal(summaryBody, &summaryWire))
	require.Equal(t, []string{"music-ovdb"}, summaryWire.EditionConnections)
	require.NotContains(t, string(summaryBody), "secret")
	require.NotContains(t, string(summaryBody), "token")
	all, err := s.LoadEnvironments(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	other, err := dalgostore.NewProjectItemsStore(db, "p", record.NewKeyWithParentAndID(record.NewKeyWithID("spaces", "two"), "projects", "p"))
	require.NoError(t, err)
	empty, err := other.LoadEnvironments(ctx)
	require.NoError(t, err)
	require.Empty(t, empty)
	require.NoError(t, s.DeleteEnvironment(ctx, "dev"))
	_, err = s.LoadEnvironment(ctx, "dev")
	require.True(t, record.IsNotFound(err))
}

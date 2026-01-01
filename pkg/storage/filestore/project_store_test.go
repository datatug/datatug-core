package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsProjectStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_project_store")
	assert.NoError(t, err)
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	projectID := "p1"
	store := newFsProjectStore(projectID, tempDir)
	ctx := context.Background()

	server1 := &datatug.ProjDbServer{
		Server: datatug.ServerReference{
			Driver: "sqlserver",
			Host:   "localhost",
			Port:   1433,
		},
	}

	t.Run("SaveProjDbServer", func(t *testing.T) {
		err := store.SaveProjDbServer(ctx, server1)
		assert.NoError(t, err)

		// Verify file exists
		serverPath := path.Join(tempDir, storage.ServersFolder, "localhost:1433."+storage.BoardFileSuffix+".json")
		_, err = os.Stat(serverPath)
		assert.NoError(t, err)
	})

	t.Run("LoadProjDbServerSummary", func(t *testing.T) {
		summary, err := store.LoadProjDbServerSummary(ctx, "localhost:1433")
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, server1.Server.Host, summary.DbServer.Host)
	})

	t.Run("LoadProjDbServers", func(t *testing.T) {
		servers, err := store.LoadProjDbServers(ctx)
		assert.NoError(t, err)
		assert.Len(t, servers, 1)
		assert.Equal(t, server1.Server.Host, servers[0].Server.Host)
	})

	t.Run("DeleteProjDbServer", func(t *testing.T) {
		err := store.DeleteProjDbServer(ctx, "localhost:1433")
		assert.NoError(t, err)

		servers, err := store.LoadProjDbServers(ctx)
		assert.NoError(t, err)
		assert.Len(t, servers, 0)
	})

	t.Run("Recordset_not_implemented", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = store.LoadRecordsetData(ctx, "r1")
		})
	})

	t.Run("EnvironmentSummary_not_found", func(t *testing.T) {
		_, err := store.LoadEnvironmentSummary(ctx, "e1")
		assert.Error(t, err)
	})

	t.Run("LoadRecordsetDefinition_not_found", func(t *testing.T) {
		_, err := store.LoadRecordsetDefinition(ctx, "r1")
		assert.Error(t, err)
	})

	t.Run("LoadRecordsetDefinitions_not_found", func(t *testing.T) {
		defs, err := store.LoadRecordsetDefinitions(ctx)
		assert.NoError(t, err)
		assert.Empty(t, defs)
	})
}

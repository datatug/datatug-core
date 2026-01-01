package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestEntitiesStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_entities")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	store := newFsEntitiesStore(tempDir)
	ctx := context.Background()

	t.Run("SaveEntity", func(t *testing.T) {
		entity := &datatug.Entity{}
		entity.ID = "e1"
		err := store.SaveEntity(ctx, entity)
		assert.NoError(t, err)

		entityPath := path.Join(tempDir, storage.EntitiesFolder, "e1."+storage.EntityFileSuffix+".json")
		assert.FileExists(t, entityPath)
	})

	t.Run("LoadEntity", func(t *testing.T) {
		entity, err := store.LoadEntity(ctx, "e1")
		assert.NoError(t, err)
		assert.NotNil(t, entity)
		assert.Equal(t, "e1", entity.ID)
	})

	t.Run("LoadEntities", func(t *testing.T) {
		entities, err := store.LoadEntities(ctx)
		assert.NoError(t, err)
		assert.Len(t, entities, 1)
	})

	t.Run("DeleteEntity", func(t *testing.T) {
		err := store.DeleteEntity(ctx, "e1")
		assert.NoError(t, err)
		entityPath := path.Join(tempDir, storage.EntitiesFolder, "e1."+storage.EntityFileSuffix+".json")
		_, err = os.Stat(entityPath)
		assert.True(t, os.IsNotExist(err))

		// Delete non-existent
		err = store.DeleteEntity(ctx, "e2")
		assert.NoError(t, err)
	})

	t.Run("SaveEntity_errors", func(t *testing.T) {
		err := store.SaveEntity(ctx, nil)
		assert.Error(t, err)

		err = store.SaveEntity(ctx, &datatug.Entity{})
		assert.Error(t, err)
	})
}

func TestQueriesStore_Saver(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_queries")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	store := newFsQueriesStore(tempDir)
	ctx := context.Background()

	t.Run("CreateQueryFolder", func(t *testing.T) {
		err := store.CreateQueryFolder(ctx, "", "f1")
		assert.NoError(t, err)
		assert.DirExists(t, path.Join(tempDir, storage.QueriesFolder, "f1"))
		assert.FileExists(t, path.Join(tempDir, storage.QueriesFolder, "f1", "README.md"))
	})

	t.Run("CreateQuery", func(t *testing.T) {
		query := datatug.QueryDefWithFolderPath{
			FolderPath: "f1",
		}
		query.ID = "q1"
		query.Title = "Query 1"
		query.Text = "SELECT 1"
		query.Type = "SQL"
		_, err := store.CreateQuery(ctx, query)
		assert.NoError(t, err)
		assert.FileExists(t, path.Join(tempDir, storage.QueriesFolder, "f1", "q1.json"))
		assert.FileExists(t, path.Join(tempDir, storage.QueriesFolder, "f1", "q1.sql"))
	})

	t.Run("CreateQueryFolder_exists", func(t *testing.T) {
		err := store.CreateQueryFolder(ctx, "", "f1")
		assert.NoError(t, err)

		// Nested
		err = store.CreateQueryFolder(ctx, "f1", "f2")
		assert.NoError(t, err)
	})

	t.Run("SaveQuery", func(t *testing.T) {
		query := &datatug.QueryDefWithFolderPath{
			FolderPath: "f1",
		}
		query.ID = "q2"
		query.Title = "Query 2"
		query.Type = "SQL"
		err := store.SaveQuery(ctx, query)
		assert.NoError(t, err)
	})
}

func TestLoadEnvServers(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_env_servers")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()

	env := &datatug.Environment{}
	serverName := "s1"
	serverFile := path.Join(tempDir, serverName+"."+storage.ServerFileSuffix+".json")
	servers := []*datatug.EnvDbServer{
		{
			ServerReference: datatug.ServerReference{
				Driver: "sqlserver",
				Host:   serverName,
			},
		},
	}
	data, _ := json.Marshal(servers)
	err = os.WriteFile(serverFile, data, 0644)
	assert.NoError(t, err)

	err = loadEnvServers(tempDir, env)
	assert.NoError(t, err)
	assert.Len(t, env.DbServers, 1)
	assert.Equal(t, serverName, env.DbServers[0].Host)

	t.Run("invalid_host", func(t *testing.T) {
		serverFile2 := path.Join(tempDir, "s2."+storage.ServerFileSuffix+".json")
		servers2 := []*datatug.EnvDbServer{
			{
				ServerReference: datatug.ServerReference{
					Driver: "sqlserver",
					Host:   "mismatch",
				},
			},
		}
		data, _ := json.Marshal(servers2)
		_ = os.WriteFile(serverFile2, data, 0644)
		err = loadEnvServers(tempDir, env)
		assert.Error(t, err)
	})
}

func TestDbServersStore_Server(t *testing.T) {
	s := fsProjectStore{}
	store := newFsDbServersStore(s)
	assert.NotNil(t, store.DbServer(datatug.ServerReference{}))
}

func TestFsDbServerStore_Catalogs(t *testing.T) {
	s := fsDbServerStore{}
	assert.NotNil(t, s.Catalogs())
}

func TestFoldersStore_LoadFolders(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_folders")
	assert.NoError(t, err)
	defer func() { _ = os.RemoveAll(tempDir) }()
	store := newFsFoldersStore(tempDir)
	_, err = store.LoadFolders(context.Background())
	assert.NoError(t, err)
}

func TestQueriesStore_DeleteQueryFolder(t *testing.T) {
	store := newFsQueriesStore("path")
	err := store.DeleteQueryFolder(context.Background(), "f1")
	assert.Error(t, err)
	assert.Equal(t, "not implemented yet", err.Error())
}

func TestDbCatalogsStore_Server(t *testing.T) {
	s := fsDbServerStore{}
	store := newFsDbCatalogsStore(s)
	assert.NotNil(t, store.Server())
	assert.NotNil(t, store.DbCatalog("c1"))
}

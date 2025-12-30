package filestore

import (
	"context"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsFoldersStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_folders")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	store := newFsFoldersStore(tmpDir)
	ctx := context.Background()

	folder1 := &datatug.Folder{
		Name: "folder1",
	}

	t.Run("SaveFolder", func(t *testing.T) {
		err := store.SaveFolder(ctx, "", folder1)
		assert.NoError(t, err)
		assert.DirExists(t, path.Join(tmpDir, FoldersDir, "folder1"))
		assert.FileExists(t, path.Join(tmpDir, FoldersDir, "folder1", ".datatug-folder.json"))
	})

	t.Run("LoadFolder", func(t *testing.T) {
		folder, err := store.LoadFolder(ctx, "folder1")
		assert.NoError(t, err)
		assert.NotNil(t, folder)
		assert.Equal(t, "folder1", folder.Name)
	})

	t.Run("SaveFolders", func(t *testing.T) {
		folder2 := &datatug.Folder{
			Name: "folder2",
		}
		err := store.SaveFolders(ctx, "", datatug.Folders{folder2})
		assert.NoError(t, err)
		assert.DirExists(t, path.Join(tmpDir, FoldersDir, "folder2"))
	})

	t.Run("DeleteFolder", func(t *testing.T) {
		err := store.DeleteFolder(ctx, "folder1")
		assert.NoError(t, err)
	})
}

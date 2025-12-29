package filestore

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestFsFoldersStore(t *testing.T) {
	store := newFsFoldersStore("/tmp/p1")
	ctx := context.Background()

	t.Run("CreateFolder_InvalidPath", func(t *testing.T) {
		err := store.SaveFolders(ctx, "", datatug.Folders{
			&datatug.Folder{Name: "folder1"},
			&datatug.Folder{Name: "folder1/folder2"},
		})
		assert.NoError(t, err)
	})

	t.Run("LoadFolder", func(t *testing.T) {
		folder, err := store.LoadFolder(ctx, "folder1")
		assert.NoError(t, err)
		assert.NotNil(t, folder)
	})

	t.Run("DeleteFolder", func(t *testing.T) {
		deleteFolder := func(id string) {
			err := store.DeleteFolder(ctx, "folder2")
			assert.NoError(t, err)
		}
		deleteFolder("folder1/folder2")
		deleteFolder("folder1")
	})
}

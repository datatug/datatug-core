package filestore

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsFoldersStore(t *testing.T) {
	store := fsFoldersStore{
		fsProjectStore: newFsProjectStore("p1", "/tmp/p1"),
	}
	ctx := context.Background()

	t.Run("CreateFolder_InvalidPath", func(t *testing.T) {
		_, err := store.CreateFolder(ctx, storage.CreateFolderRequest{Path: ""})
		assert.Error(t, err)
	})

	//t.Run("CreateFolder_Panic", func(t *testing.T) {
	//	assert.Panics(t, func() {
	//		_, _ = store.CreateFolder(ctx, storage.CreateFolderRequest{Path: "valid/path"})
	//	})
	//})

	t.Run("GetFolder", func(t *testing.T) {
		assert.Panics(t, func() {
			_, _ = store.GetFolder(ctx, "path")
		})
	})

	t.Run("DeleteFolder", func(t *testing.T) {
		assert.Panics(t, func() {
			_ = store.DeleteFolder(ctx, "path")
		})
	})
}

package filestore

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsProjectStore_ProjectID(t *testing.T) {
	const projectID = "p1"
	store := fsProjectStore{projectID: projectID}
	assert.Equal(t, projectID, store.ProjectID())
}

func TestNewStore(t *testing.T) {
	id := "test-store"
	paths := map[string]string{"p1": "/path/p1"}
	store, err := NewStore(id, paths)
	assert.NoError(t, err)
	fsStore := store.(*FsStore)
	assert.Equal(t, id, fsStore.id)
	assert.Equal(t, paths, fsStore.pathByID)
}

func TestNewSingleProjectStore(t *testing.T) {
	t.Run("with_id", func(t *testing.T) {
		path := "/path/p1"
		id := "p1"
		store, projID := NewSingleProjectStore(path, id)
		assert.Equal(t, id, projID)
		assert.Equal(t, path, store.pathByID[projID])
	})

	t.Run("without_id", func(t *testing.T) {
		path := "/path/p1"
		store, projID := NewSingleProjectStore(path, "")
		assert.Equal(t, storage.SingleProjectID, projID)
		assert.Equal(t, path, store.pathByID[projID])
	})
}

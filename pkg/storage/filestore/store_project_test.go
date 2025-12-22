package filestore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFsStore_GetProjectStore(t *testing.T) {
	store := &FsStore{
		id: "test",
		pathByID: map[string]string{
			"p1": "/path/p1",
		},
	}
	ps := store.GetProjectStore("p1")
	assert.NotNil(t, ps)
	assert.Equal(t, "p1", ps.ProjectID())
}

func TestFsStore_DeleteProject(t *testing.T) {
	store := &FsStore{}
	err := store.DeleteProject(context.Background(), "p1")
	assert.Error(t, err)
}

func TestFsProjectStore_SubStores(t *testing.T) {
	ps := newFsProjectStore("p1", "/path/p1")
	assert.NotNil(t, ps.DbModels())
	assert.NotNil(t, ps.Environments())
	assert.NotNil(t, ps.DbServers())
	assert.NotNil(t, ps.Folders())
	assert.NotNil(t, ps.Boards())
	assert.NotNil(t, ps.Queries())
}

func TestFsProjectStoreRef_Project(t *testing.T) {
	ps := newFsProjectStore("p1", "/path/p1")
	ref := fsProjectStoreRef{fsProjectStore: ps}
	assert.Equal(t, ps, ref.Project())
}

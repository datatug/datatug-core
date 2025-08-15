package filestore

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestFsProjectStore_ProjectID(t *testing.T) {
	const projectID = "p1"
	store := fsProjectStore{projectID: projectID}
	assert.Equal(t, projectID, store.ProjectID())
}

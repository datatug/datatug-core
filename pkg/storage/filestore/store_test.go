package filestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFsProjectStore_ProjectID(t *testing.T) {
	const projectID = "p1"
	store := fsProjectStore{projectID: projectID}
	assert.Equal(t, projectID, store.ProjectID())
}

package filestore

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewFsProjectStore(t *testing.T) {
	var tempDir string
	{
		var err error
		tempDir, err = os.MkdirTemp("", "datatug_test_project_store")
		assert.NoError(t, err)
		defer func(path string) {
			_ = os.RemoveAll(path)
		}(tempDir)
	}

	projectID := "p1"
	store := newFsProjectStore(projectID, tempDir)
	assert.NotNil(t, store)
}

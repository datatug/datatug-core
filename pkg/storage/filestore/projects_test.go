package filestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetProjectPath(t *testing.T) {
	const testID = "test-id"
	var v = GetProjectPath(testID)
	assert.NotNil(t, v)
	assert.Equal(t, "", v)
}

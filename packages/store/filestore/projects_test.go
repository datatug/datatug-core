package filestore

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestGetProjectPath(t *testing.T) {
	const testID = "test-id"
	var v = GetProjectPath(testID)
	assert.NotNil(t, v)
	assert.Equal(t, "", v)
}

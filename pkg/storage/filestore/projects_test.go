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

func TestSetProjectPath(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		id := "p1"
		path := "/path/p1"
		SetProjectPath(id, path)
		if GetProjectPath(id) != path {
			t.Errorf("expected path %v, got %v", path, GetProjectPath(id))
		}
	})

	t.Run("empty_id", func(t *testing.T) {
		assert.Panics(t, func() {
			SetProjectPath("", "/path")
		})
	})

	t.Run("empty_path", func(t *testing.T) {
		assert.Panics(t, func() {
			SetProjectPath("p2", "")
		})
	})

	t.Run("overwrite_panic", func(t *testing.T) {
		id := "p3"
		SetProjectPath(id, "/path/1")
		assert.Panics(t, func() {
			SetProjectPath(id, "/path/2")
		})
	})

	t.Run("duplicate_ok", func(t *testing.T) {
		id := "p4"
		path := "/path/4"
		SetProjectPath(id, path)
		assert.NotPanics(t, func() {
			SetProjectPath(id, path)
		})
	})
}

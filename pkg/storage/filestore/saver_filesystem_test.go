package filestore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockValidatable struct {
	ID    string `json:"id"`
	Error error  `json:"-"`
}

func (m mockValidatable) Validate() error {
	return m.Error
}

func TestSaveJSONFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_save")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Run("valid", func(t *testing.T) {
		v := mockValidatable{ID: "test-id"}
		err := saveJSONFile(tmpDir, "test.json", v)
		assert.NoError(t, err)

		filePath := filepath.Join(tmpDir, "test.json")
		data, err := os.ReadFile(filePath)
		assert.NoError(t, err)

		var loaded mockValidatable
		err = json.Unmarshal(data, &loaded)
		assert.NoError(t, err)
		assert.Equal(t, "test-id", loaded.ID)
	})

	t.Run("invalid", func(t *testing.T) {
		v := mockValidatable{Error: assert.AnError}
		err := saveJSONFile(tmpDir, "invalid.json", v)
		assert.Error(t, err)
	})

	t.Run("mkdir_fail", func(t *testing.T) {
		// Create a file where we want a directory
		filePath := filepath.Join(tmpDir, "somefile")
		_ = os.WriteFile(filePath, []byte("test"), 0644)

		v := mockValidatable{ID: "test"}
		err := saveJSONFile(filepath.Join(filePath, "subdir"), "test.json", v)
		assert.Error(t, err)
	})
}

func TestSaveItems(t *testing.T) {
	t.Run("zero_items", func(t *testing.T) {
		err := saveItems("items", 0, nil)
		assert.NoError(t, err)
	})

	t.Run("one_item", func(t *testing.T) {
		called := false
		err := saveItems("items", 1, func(i int) func() error {
			return func() error {
				called = true
				return nil
			}
		})
		assert.NoError(t, err)
		assert.True(t, called)
	})

	t.Run("multiple_items", func(t *testing.T) {
		count := 3
		called := make([]bool, count)
		err := saveItems("items", count, func(i int) func() error {
			return func() error {
				called[i] = true
				return nil
			}
		})
		assert.NoError(t, err)
		for i := 0; i < count; i++ {
			assert.True(t, called[i])
		}
	})

	t.Run("error_item", func(t *testing.T) {
		err := saveItems("items", 2, func(i int) func() error {
			return func() error {
				if i == 1 {
					return assert.AnError
				}
				return nil
			}
		})
		assert.Error(t, err)
	})
}

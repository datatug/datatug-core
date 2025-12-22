package filestore

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDirExists(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Run("exists", func(t *testing.T) {
		exists, err := DirExists(tmpDir)
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("not_exists", func(t *testing.T) {
		exists, err := DirExists(filepath.Join(tmpDir, "non_existent"))
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("is_file", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "file.txt")
		err := os.WriteFile(filePath, []byte("test"), 0644)
		assert.NoError(t, err)

		exists, err := DirExists(filePath)
		assert.NoError(t, err)
		assert.False(t, exists)
	})
}

func TestExpandHome(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Equal(t, "", ExpandHome(""))
	})
	t.Run("no_tilde", func(t *testing.T) {
		assert.Equal(t, "/some/path", ExpandHome("/some/path"))
	})
	t.Run("only_tilde", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		assert.Equal(t, home, ExpandHome("~"))
	})
	t.Run("tilde_with_path", func(t *testing.T) {
		home, _ := os.UserHomeDir()
		assert.Equal(t, filepath.Join(home, "abc"), ExpandHome("~/abc"))
	})
}

package filestore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStorage(t *testing.T) {
	s := NewStorage("test")
	assert.NotNil(t, s)
	fs, ok := s.(fsStorage)
	assert.True(t, ok)
	assert.Equal(t, "test", fs.projPath)
}

func TestFsStorage(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.RemoveAll(tempDir)
	}()

	storage := NewStorage(tempDir)
	ctx := context.Background()

	t.Run("WriteFile", func(t *testing.T) {
		content := "test content"
		err := storage.WriteFile(ctx, "test.txt", strings.NewReader(content))
		assert.NoError(t, err)

		// Verify file exists on disk
		data, err := os.ReadFile(filepath.Join(tempDir, "test.txt"))
		assert.NoError(t, err)
		assert.Equal(t, content, string(data))
	})

	t.Run("FileExists", func(t *testing.T) {
		exists, err := storage.FileExists(ctx, "test.txt")
		assert.NoError(t, err)
		assert.True(t, exists)

		exists, err = storage.FileExists(ctx, "non-existent.txt")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("OpenFile", func(t *testing.T) {
		reader, err := storage.OpenFile(ctx, "test.txt")
		assert.NoError(t, err)
		assert.NotNil(t, reader)
		defer func() {
			_ = reader.Close()
		}()

		data, err := io.ReadAll(reader)
		assert.NoError(t, err)
		assert.Equal(t, "test content", string(data))
	})

	t.Run("WriteFile_InSubdir", func(t *testing.T) {
		content := "subdir content"
		err := storage.WriteFile(ctx, "subdir/test.txt", strings.NewReader(content))
		assert.NoError(t, err)

		exists, err := storage.FileExists(ctx, "subdir/test.txt")
		assert.NoError(t, err)
		assert.True(t, exists)
	})
}

func Test_fsStorage_Commit(t *testing.T) {
	err := fsStorage{}.Commit(context.Background(), "test")
	assert.NoError(t, err)
}

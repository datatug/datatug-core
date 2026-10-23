//go:build unix

package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestProjectDataModelsMetadataFilesystemFailures(t *testing.T) {
	t.Run("invalid incoming version", func(t *testing.T) {
		s := newFsProjectStore("p", t.TempDir())
		_, _, err := s.preflightProjectDataModels(&datatug.ProjectDataModels{Version: 2})
		require.ErrorIs(t, err, datatug.ErrUnsupportedDataModels)
	})
	t.Run("summary cannot be read", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, storage.ProjectSummaryFileName)
		require.NoError(t, os.WriteFile(p, []byte(`{}`), 0600))
		require.NoError(t, os.Chmod(p, 0))
		defer func() { _ = os.Chmod(p, 0600) }()
		_, _, err := newFsProjectStore("p", dir).preflightProjectDataModels(nil)
		require.Error(t, err)
	})
	t.Run("metadata path cannot be inspected", func(t *testing.T) {
		_, _, err := newFsProjectStore("p", filepath.Join(t.TempDir(), strings.Repeat("a", 256))).preflightProjectDataModels(nil)
		require.Error(t, err)
	})
	t.Run("project directory cannot be created", func(t *testing.T) {
		parent := t.TempDir()
		require.NoError(t, os.Chmod(parent, 0500))
		defer func() { _ = os.Chmod(parent, 0700) }()
		s := newFsProjectStore("p", filepath.Join(parent, "new"))
		err := s.saveProjectUnlocked(context.Background(), modelProject())
		require.ErrorContains(t, err, "failed to create datatug folder")
	})
}

package filestore

import (
	"context"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestReadOnlyEnvironmentProjectDoesNotInitializeState(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{"id":"p","title":"Project"}`), 0400))
	require.NoError(t, os.Chmod(root, 0500))
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	file, err := LoadProjectFile(root)
	require.NoError(t, err)
	require.Equal(t, "Project", file.Title)
	require.NoError(t, ReadEnvironmentFiles(context.Background(), root, func(f *EnvironmentFiles) error {
		_, err := f.LoadEnvironments(context.Background())
		return err
	}))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestReadOnlyEnvironmentProjectRejectsLostOwner(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".datatug-environments.lock"), []byte("environment-owner/1:lost:lost"), 0600))
	called := false
	err := ReadEnvironmentFiles(context.Background(), root, func(*EnvironmentFiles) error { called = true; return nil })
	require.ErrorIs(t, err, ErrEnvironmentRecoveryRequired)
	require.False(t, called)
}

package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFsEnvironmentsStore(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "datatug_test_environments")
	assert.NoError(t, err)
	defer func(path string) {
		_ = os.RemoveAll(path)
	}(tempDir)

	store := newFsEnvironmentsStore(tempDir)

	ctx := context.Background()
	env1 := &datatug.Environment{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    "env1",
				Title: "Environment 1",
			},
		},
	}

	t.Run("SaveEnvironment", func(t *testing.T) {
		err := store.SaveEnvironment(ctx, env1)
		assert.NoError(t, err)

		// A brand-new environment defaults to the demo's own filename
		// ("<id>.env.json"), matching datatug-demo-projects.
		envPath := path.Join(tempDir, storage.EnvironmentsFolder, "env1", "env1.env.json")
		_, err = os.Stat(envPath)
		assert.NoError(t, err)
	})

	t.Run("LoadEnvironment", func(t *testing.T) {
		loadedEnv, err := store.LoadEnvironment(ctx, "env1")
		assert.NoError(t, err)
		assert.Equal(t, env1.ID, loadedEnv.ID)
		assert.Equal(t, env1.Title, loadedEnv.Title)
	})

	t.Run("LoadEnvironmentSummary", func(t *testing.T) {
		summary, err := store.LoadEnvironmentSummary(ctx, "env1")
		assert.NoError(t, err)
		assert.NotNil(t, summary)
		assert.Equal(t, env1.ID, summary.ID)
	})

	t.Run("LoadEnvironments", func(t *testing.T) {
		envs, err := store.LoadEnvironments(ctx)
		assert.NoError(t, err)
		assert.Len(t, envs, 1)
		assert.Equal(t, env1.ID, envs[0].ID)
	})

	t.Run("SaveEnvironments", func(t *testing.T) {
		env2 := &datatug.Environment{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID:    "env2",
					Title: "Environment 2",
				},
			},
		}
		err := store.SaveEnvironments(ctx, datatug.Environments{env1, env2})
		assert.NoError(t, err)

		envs, err := store.LoadEnvironments(ctx)
		assert.NoError(t, err)
		assert.Len(t, envs, 2)
	})

	t.Run("DeleteEnvironment", func(t *testing.T) {
		err := store.DeleteEnvironment(ctx, "env1")
		assert.NoError(t, err)

		envDir := path.Join(tempDir, storage.EnvironmentsFolder, "env1")
		assert.NoDirExists(t, envDir, "DeleteEnvironment must remove the whole per-environment directory")

		// Deleting an absent environment is a no-op, not an error.
		assert.NoError(t, store.DeleteEnvironment(ctx, "never-existed"))
	})
}

// TestFsEnvironmentsStore_DualLayout covers the layout-compatibility rules
// LoadEnvironment(s) must apply, mirroring S27/S28's entities/boards/dbmodels
// fixes: environments live in a per-id directory either way, but the file
// inside it is named either the legacy "environment-summary.json" or the
// demo's own "<id>.env.json" (datatug-demo-projects/demo-project-1/
// environments/local/local.env.json) - the demo's own filename wins when
// both exist and agree; disagreement is a clear error; a brand-new
// environment defaults to the demo's filename.
func TestFsEnvironmentsStore_DualLayout(t *testing.T) {
	newStore := func(t *testing.T) (fsEnvironmentsStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_environments_layout_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		envsDir := path.Join(tmpDir, storage.EnvironmentsFolder)
		require.NoError(t, os.MkdirAll(envsDir, 0777))
		return newFsEnvironmentsStore(tmpDir), envsDir
	}

	writeLegacy := func(t *testing.T, envsDir string, env *datatug.Environment) {
		t.Helper()
		dir := path.Join(envsDir, env.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(env)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path.Join(dir, storage.EnvironmentSummaryFileName), data, 0644))
	}
	writeDemo := func(t *testing.T, envsDir string, env *datatug.Environment) {
		t.Helper()
		dir := path.Join(envsDir, env.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(env)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(path.Join(dir, env.ID+".env.json"), data, 0644))
	}

	t.Run("loads_legacy_only", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEnvironment(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "e1", e.ID)
	})

	t.Run("loads_demo_only", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeDemo(t, envsDir, &datatug.Environment{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "local"}},
			DbServers:   datatug.EnvDbServers{{ServerRef: datatug.ServerRef{Driver: "sqlite3"}, Catalogs: []string{"chinook-local"}}},
		})

		e, err := store.LoadEnvironment(context.Background(), "local")
		assert.NoError(t, err)
		assert.Equal(t, "local", e.ID)
		require.Len(t, e.DbServers, 1)
		assert.Equal(t, []string{"chinook-local"}, e.DbServers[0].Catalogs)
	})

	t.Run("demo_wins_when_both_agree", func(t *testing.T) {
		store, envsDir := newStore(t)
		env := &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}}
		writeLegacy(t, envsDir, env)
		writeDemo(t, envsDir, env)

		e, err := store.LoadEnvironment(context.Background(), "e1")
		assert.NoError(t, err)
		assert.Equal(t, "e1", e.ID)
	})

	t.Run("conflicting_duplicate_is_a_clear_error", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Legacy"}}})
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Demo"}}})

		_, err := store.LoadEnvironment(context.Background(), "e1")
		assert.Error(t, err)
	})

	t.Run("not_found_in_either_layout", func(t *testing.T) {
		store, envsDir := newStore(t)
		require.NoError(t, os.MkdirAll(path.Join(envsDir, "empty"), 0777))

		_, err := store.LoadEnvironment(context.Background(), "empty")
		assert.Error(t, err)
	})

	t.Run("loadEnvironments_merges_both_layouts_deduped_and_sorted", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "zzz"}}})
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "aaa"}}})

		envs, err := store.LoadEnvironments(context.Background())
		require.NoError(t, err)
		require.Len(t, envs, 2)
		assert.Equal(t, "aaa", envs[0].ID)
		assert.Equal(t, "zzz", envs[1].ID)
	})

	t.Run("loadEnvironmentSummary_accepts_the_demo_layout", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeDemo(t, envsDir, &datatug.Environment{
			ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "local"}},
			DbServers:   datatug.EnvDbServers{{ServerRef: datatug.ServerRef{Driver: "sqlite3"}}},
		})

		summary, err := store.LoadEnvironmentSummary(context.Background(), "local")
		assert.NoError(t, err)
		require.NotNil(t, summary)
		assert.Equal(t, "local", summary.ID)
		assert.Len(t, summary.Servers, 1)
	})

	t.Run("save_preserves_legacy_layout_it_was_loaded_from", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEnvironment(context.Background(), "e1")
		require.NoError(t, err)
		e.Title = "Updated"
		require.NoError(t, store.SaveEnvironment(context.Background(), e))

		assert.FileExists(t, path.Join(envsDir, "e1", storage.EnvironmentSummaryFileName))
		assert.NoFileExists(t, path.Join(envsDir, "e1", "e1.env.json"))
	})

	t.Run("save_preserves_demo_layout_it_was_loaded_from", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})

		e, err := store.LoadEnvironment(context.Background(), "e1")
		require.NoError(t, err)
		e.Title = "Updated"
		require.NoError(t, store.SaveEnvironment(context.Background(), e))

		assert.FileExists(t, path.Join(envsDir, "e1", "e1.env.json"))
		assert.NoFileExists(t, path.Join(envsDir, "e1", storage.EnvironmentSummaryFileName))
	})

	t.Run("save_new_defaults_to_demo_layout", func(t *testing.T) {
		store, envsDir := newStore(t)
		require.NoError(t, store.SaveEnvironment(context.Background(), &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}}))

		assert.FileExists(t, path.Join(envsDir, "e1", "e1.env.json"))
	})

	t.Run("malformed_demo_file_is_a_clear_error", func(t *testing.T) {
		store, envsDir := newStore(t)
		dir := path.Join(envsDir, "e1")
		require.NoError(t, os.MkdirAll(dir, 0777))
		require.NoError(t, os.WriteFile(path.Join(dir, "e1.env.json"), []byte("not json"), 0644))

		_, err := store.LoadEnvironment(context.Background(), "e1")
		assert.Error(t, err)
	})

	t.Run("malformed_legacy_file_is_a_clear_error", func(t *testing.T) {
		store, envsDir := newStore(t)
		dir := path.Join(envsDir, "e1")
		require.NoError(t, os.MkdirAll(dir, 0777))
		require.NoError(t, os.WriteFile(path.Join(dir, storage.EnvironmentSummaryFileName), []byte("not json"), 0644))

		_, err := store.LoadEnvironment(context.Background(), "e1")
		assert.Error(t, err)
	})

	t.Run("loadEnvironments_propagates_a_conflicting_duplicate_error", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "ok"}}})
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Legacy"}}})
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Demo"}}})

		_, err := store.LoadEnvironments(context.Background())
		assert.Error(t, err)
	})

	t.Run("loadEnvironments_skips_a_directory_with_neither_file", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1"}}})
		require.NoError(t, os.MkdirAll(path.Join(envsDir, "empty"), 0777))

		envs, err := store.LoadEnvironments(context.Background())
		require.NoError(t, err)
		require.Len(t, envs, 1)
		assert.Equal(t, "e1", envs[0].ID)
	})

	t.Run("loadEnvironmentSummary_not_found", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.LoadEnvironmentSummary(context.Background(), "missing")
		assert.Error(t, err)
	})

	t.Run("loadEnvironmentSummary_propagates_a_conflicting_duplicate_error", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Legacy"}}})
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "e1", Title: "Demo"}}})

		_, err := store.LoadEnvironmentSummary(context.Background(), "e1")
		assert.Error(t, err)
	})

	t.Run("saveEnvironment_propagates_invalid_data_error", func(t *testing.T) {
		store, _ := newStore(t)
		err := store.SaveEnvironment(context.Background(), &datatug.Environment{})
		assert.Error(t, err)
	})

	t.Run("delete_removes_the_whole_directory_for_either_layout", func(t *testing.T) {
		store, envsDir := newStore(t)
		writeLegacy(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "legacy1"}}})
		writeDemo(t, envsDir, &datatug.Environment{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "demo1"}}})

		require.NoError(t, store.DeleteEnvironment(context.Background(), "legacy1"))
		require.NoError(t, store.DeleteEnvironment(context.Background(), "demo1"))

		assert.NoDirExists(t, path.Join(envsDir, "legacy1"))
		assert.NoDirExists(t, path.Join(envsDir, "demo1"))
	})
}

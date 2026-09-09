package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.EnvironmentsStore = (*fsEnvironmentsStore)(nil)

func newFsEnvironmentsStore(projectPath string) fsEnvironmentsStore {
	return fsEnvironmentsStore{
		fsProjectItemsStore: newDirProjectItemsStore[datatug.Environments, *datatug.Environment, datatug.Environment](
			path.Join(projectPath, storage.EnvironmentsFolder), storage.EnvironmentSummaryFileName,
		),
	}
}

// fsEnvironmentsStore loads and saves environments from two on-disk
// filenames within the same per-environment directory: the legacy
// "environment-summary.json" and the demo's own "<id>.env.json" (e.g.
// datatug-demo-projects/demo-project-1/environments/local/local.env.json).
// The demo's filename wins when both exist and agree; disagreement is a
// clear load error. A save keeps the filename an environment was loaded
// from; a brand-new environment defaults to the demo's filename.
type fsEnvironmentsStore struct {
	fsProjectItemsStore[datatug.Environments, *datatug.Environment, datatug.Environment]
}

func (s fsEnvironmentsStore) legacyEnvFilePath(id string) string {
	return path.Join(s.dirPath, id, storage.EnvironmentSummaryFileName)
}

func (s fsEnvironmentsStore) demoEnvFilePath(id string) string {
	return path.Join(s.dirPath, id, storage.JsonFileName(id, storage.EnvFileSuffix))
}

func readEnvironmentFile(filePath string) (*datatug.Environment, error) {
	env := new(datatug.Environment)
	if err := readJSONFile(filePath, true, env); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return env, nil
}

// loadOneEnvironment reads id from both filenames and reconciles them: the
// demo's filename wins when both exist and their content matches; a content
// mismatch is a clear error. Returns (nil, nil) when the environment exists
// in neither.
func (s fsEnvironmentsStore) loadOneEnvironment(id string) (*datatug.Environment, error) {
	demoPath, legacyPath := s.demoEnvFilePath(id), s.legacyEnvFilePath(id)
	demo, err := readEnvironmentFile(demoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load environment[%s] from %s: %w", id, demoPath, err)
	}
	legacy, err := readEnvironmentFile(legacyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load environment[%s] from %s: %w", id, legacyPath, err)
	}
	switch {
	case demo != nil && legacy != nil:
		if !reflect.DeepEqual(demo, legacy) {
			return nil, fmt.Errorf(
				"environment[%s] is stored in both %s and %s with different content; remove one",
				id, demoPath, legacyPath)
		}
		demo.SetID(id)
		return demo, nil
	case demo != nil:
		demo.SetID(id)
		return demo, nil
	case legacy != nil:
		legacy.SetID(id)
		return legacy, nil
	default:
		return nil, nil
	}
}

func (s fsEnvironmentsStore) LoadEnvironment(_ context.Context, id string, o ...datatug.StoreOption) (*datatug.Environment, error) {
	_ = datatug.GetStoreOptions(o...)
	env, err := s.loadOneEnvironment(id)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, fmt.Errorf("failed to load environment[%s] from project: %w", id, os.ErrNotExist)
	}
	return env, nil
}

func (s fsEnvironmentsStore) LoadEnvironments(_ context.Context, o ...datatug.StoreOption) (datatug.Environments, error) {
	_ = datatug.GetStoreOptions(o...)
	dirEntries, err := os.ReadDir(s.dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var envs datatug.Environments
	for _, de := range dirEntries {
		if !de.IsDir() {
			continue
		}
		env, err := s.loadOneEnvironment(de.Name())
		if err != nil {
			return nil, err
		}
		if env == nil {
			continue // a directory with neither environment file
		}
		envs = append(envs, env)
	}
	sort.Slice(envs, func(i, j int) bool {
		return envs[i].GetID() < envs[j].GetID()
	})
	return envs, nil
}

func (s fsEnvironmentsStore) LoadEnvironmentSummary(_ context.Context, id string) (*datatug.EnvironmentSummary, error) {
	env, err := s.loadOneEnvironment(id)
	if err != nil {
		return nil, err
	}
	if env == nil {
		return nil, fmt.Errorf("failed to load environment[%s] from project: %w", id, os.ErrNotExist)
	}
	return &datatug.EnvironmentSummary{ProjectItem: env.ProjectItem, Servers: env.DbServers}, nil
}

// saveTarget mirrors the entities/boards/dbmodels stores' saveTarget: the
// demo's filename if it already exists (demo wins, same as
// loadOneEnvironment), else the legacy filename if that already exists
// (preserve the filename an environment was loaded from), else the demo's
// filename as the default for a brand-new environment.
func (s fsEnvironmentsStore) saveTarget(id string) (dirPath, fileName string) {
	dirPath = path.Join(s.dirPath, id)
	if _, err := os.Stat(s.demoEnvFilePath(id)); err == nil {
		return dirPath, storage.JsonFileName(id, storage.EnvFileSuffix)
	}
	if _, err := os.Stat(s.legacyEnvFilePath(id)); err == nil {
		return dirPath, storage.EnvironmentSummaryFileName
	}
	return dirPath, storage.JsonFileName(id, storage.EnvFileSuffix)
}

func (s fsEnvironmentsStore) SaveEnvironment(_ context.Context, env *datatug.Environment) error {
	dirPath, fileName := s.saveTarget(env.ID)
	if err := saveJSONFile(dirPath, fileName, env); err != nil {
		return fmt.Errorf("failed to save environment file: %w", err)
	}
	return nil
}

func (s fsEnvironmentsStore) SaveEnvironments(ctx context.Context, envs datatug.Environments) error {
	return saveItems(s.dirPath, len(envs), func(i int) func() error {
		return func() error {
			return s.SaveEnvironment(ctx, envs[i])
		}
	})
}

// DeleteEnvironment removes the whole per-environment directory: an
// environment is always directory-based (both filenames live inside
// environments/<id>/), so deleting one means removing that directory,
// whichever filename(s) it holds.
func (s fsEnvironmentsStore) DeleteEnvironment(_ context.Context, id string) error {
	dirPath := path.Join(s.dirPath, id)
	if _, err := os.Stat(dirPath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dirPath)
}

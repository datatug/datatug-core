package filestore

import (
	"context"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s fsProjectStore) loadEnvironments(_ context.Context, o ...datatug.StoreOption) (environments datatug.Environments, err error) {
	envsDirPath := path.Join(s.projectPath, DatatugFolder, EnvironmentsFolder)
	err = loadDir(nil, envsDirPath, "", processDirs,
		func(files []os.FileInfo) {
			environments = make(datatug.Environments, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) (err error) {
			env := new(datatug.Environment)
			env.ID = f.Name()
			mutex.Lock()
			environments = append(environments, env)
			mutex.Unlock()
			if err = s.loadEnvironment(path.Join(envsDirPath, env.ID), env, o...); err != nil {
				return err
			}
			return
		})
	if err != nil {
		return
	}
	// Sort environments by ID for a consistent order
	sort.Slice(environments, func(i, j int) bool {
		return strings.ToLower(environments[i].ID) < strings.ToLower(environments[j].ID)
	})
	return
}

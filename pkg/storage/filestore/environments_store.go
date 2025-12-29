package filestore

import (
	"context"
	"log"
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
			if err = s.loadEnvironment(path.Join(envsDirPath, env.ID), env, o...); err != nil {
				log.Printf("failed to load environment [%v]: %v", env.ID, err)
				return err
			}
			mutex.Lock()
			environments = append(environments, env)
			mutex.Unlock()
			return
		})
	if err != nil {
		return
	}
	// Sort environments by GetID for a consistent order
	sort.Slice(environments, func(i, j int) bool {
		return strings.ToLower(environments[i].ID) < strings.ToLower(environments[j].ID)
	})
	return
}

package filestore

import (
	"log"
	"sync"
)

var projectPaths = make(map[string]string, 1) // TODO(refactoring): global package level states are bad
var projectPathsMu sync.RWMutex

// GetProjectPath gets project projDirPath
func GetProjectPath(id string) string {
	projectPathsMu.RLock()
	defer projectPathsMu.RUnlock()
	return projectPaths[id]
}

// SetProjectPath sets project projDirPath
func SetProjectPath(id, path string) {
	if id == "" {
		panic("id is a required parameter")
	}
	if path == "" {
		panic("projDirPath is a required parameter")
	}
	projectPathsMu.Lock()
	defer projectPathsMu.Unlock()
	if p, ok := projectPaths[id]; ok {
		if p != path {
			panic("attempt to overwrite project projDirPath")
		}
		log.Printf("Duplicate set of project projDirPath %v: %v", id, path)
	}
	projectPaths[id] = path
}

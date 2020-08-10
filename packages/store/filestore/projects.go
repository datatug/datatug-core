package filestore

import (
	"log"
)

var projectPaths = make(map[string]string, 1) // TODO(refactoring): global package level states are bad

// GetProjectPath gets project path
func GetProjectPath(id string) string {
	return projectPaths[id]
}

// SetProjectPath sets project path
func SetProjectPath(id, path string) {
	if id == "" {
		panic("id is a required parameter")
	}
	if path == "" {
		panic("path is a required parameter")
	}
	if p, ok := projectPaths[id]; ok {
		if p != path {
			panic("attempt to overwrite project path")
		} else {
			log.Printf("Duplicate set of projcet path %v: %v", id, path)
		}
	}
	projectPaths[id] = path
}

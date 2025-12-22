package filestore

import (
	"testing"
)

func TestNewProjectsLoader(t *testing.T) {
	path := "/some/path"
	loader := NewProjectsLoader(path)
	pl, ok := loader.(*projectsLoader)
	if !ok {
		t.Fatalf("expected *projectsLoader, got %T", loader)
	}
	if pl.projectsDirPath != path {
		t.Errorf("expected projectsDirPath %v, got %v", path, pl.projectsDirPath)
	}
}

func TestNewProjectLoader(t *testing.T) {
	path := "/some/project/path"
	loader := NewProjectLoader(path)
	pl, ok := loader.(*projectLoader)
	if !ok {
		t.Fatalf("expected *projectLoader, got %T", loader)
	}
	if pl.projectDir != path {
		t.Errorf("expected projectDir %v, got %v", path, pl.projectDir)
	}
}

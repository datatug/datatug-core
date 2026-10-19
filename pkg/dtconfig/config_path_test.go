package dtconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitConfigPathIsolatesSettings(t *testing.T) {
	SetConfigFilePath("")
	defaultPath := GetConfigFilePath()
	t.Cleanup(func() { SetConfigFilePath("") })
	selected := filepath.Join(t.TempDir(), "isolated.yaml")
	SetConfigFilePath(selected)
	if got := GetConfigFilePath(); got != selected {
		t.Fatalf("path = %q, want %q", got, selected)
	}
	if _, err := GetSettings(); !os.IsNotExist(err) {
		t.Fatalf("missing isolated file: %v", err)
	}
	want := Settings{Projects: []*ProjectRef{{ID: "demo", Path: "/tmp/demo-project"}}}
	if err := SaveSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := GetSettings()
	if err != nil || len(got.Projects) != 1 || *got.Projects[0] != *want.Projects[0] {
		t.Fatalf("isolated roundtrip: %+v, %v", got, err)
	}
	SetConfigFilePath("")
	if got := GetConfigFilePath(); got != defaultPath {
		t.Fatalf("default path changed: %q, want %q", got, defaultPath)
	}
}

package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func environmentProjectFile() datatug.ProjectFile {
	file := datatug.ProjectFile{}
	file.ID = "p"
	file.Title = "Project"
	file.Access = "public"
	file.Created = &datatug.ProjectCreated{At: time.Now()}
	return file
}
func TestEnvironmentProjectPreservesExtraMetadata(t *testing.T) {
	root := environmentTestDir(t)
	store := newFsProjectStore("p", root)
	target := filepath.Join(root, "datatug-project.json")
	if err := os.WriteFile(target, []byte(`{"title":"Old","unknown":{"important":true},"environmentOrder":["prod"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	file := environmentProjectFile()
	file.EnvironmentOrder = []string{"dev", "prod"}
	if err := store.putProjectFile(file); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["unknown"]) != `{"important":true}` || string(fields["environmentOrder"]) != `["dev","prod"]` {
		t.Fatalf("metadata overwritten %s", body)
	}
}
func TestEnvironmentProjectWriteFailures(t *testing.T) {
	for _, kind := range []string{"symlink", "directory", "corrupt", "marshal"} {
		t.Run(kind, func(t *testing.T) {
			root := environmentTestDir(t)
			store := newFsProjectStore("p", root)
			target := filepath.Join(root, "datatug-project.json")
			file := environmentProjectFile()
			switch kind {
			case "symlink":
				if err := os.Symlink(filepath.Join(root, "outside"), target); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
			case "corrupt":
				if err := os.WriteFile(target, []byte(`{"broken":`), 0600); err != nil {
					t.Fatal(err)
				}
			case "marshal":
				file.Created.At = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			if err := store.putProjectFile(file); err == nil {
				t.Fatal("unsafe project write succeeded")
			}
		})
	}
	store := newFsProjectStore("p", t.TempDir())
	if err := store.saveProjectUnlocked(context.Background(), &datatug.Project{}); err == nil {
		t.Fatal("invalid project accepted")
	}
	project := &datatug.Project{}
	project.ID = "p"
	project.Title = "Project"
	project.Access = "public"
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	store = newFsProjectStore("p", filepath.Join(blocker, "child"))
	if err := store.saveProjectUnlocked(context.Background(), project); err == nil {
		t.Fatal("non-directory project saved")
	}
}
func TestEnvironmentProjectSymlinkReads(t *testing.T) {
	root := environmentTestDir(t)
	target := filepath.Join(root, "datatug-project.json")
	if err := os.Symlink(filepath.Join(root, "outside"), target); err != nil {
		t.Fatal(err)
	}
	if err := loadProjectFile(root, &datatug.Project{}); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
	if _, err := loadProjectFileUnlocked(root); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
	envDir := filepath.Join(root, "environments", "dev")
	if err := os.MkdirAll(envDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(envDir, "dev.env.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := readEnvironmentFile(filepath.Join(envDir, "dev.env.json")); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
}
func TestEnvironmentListMalformedProjectOrder(t *testing.T) {
	for _, body := range []string{`{"environmentOrder":1}`} {
		root := environmentTestDir(t)
		if err := os.Mkdir(filepath.Join(root, "environments"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := newFsEnvironmentsStore(root).loadEnvironments(context.Background()); err == nil {
			t.Fatalf("invalid project order accepted: %s", body)
		}
	}
}

type environmentInvalidDirEntry struct{ os.DirEntry }

func (e environmentInvalidDirEntry) Name() string { return string([]byte{255}) }
func (e environmentInvalidDirEntry) IsDir() bool  { return true }
func TestEnvironmentListRejectsNonUnicodeDirectoryID(t *testing.T) {
	// POSIX filesystems can expose arbitrary filename bytes. macOS refuses such
	// names, so adapt the directory/read boundary while exercising the same loader.
	root := environmentTestDir(t)
	dir := filepath.Join(root, "environments")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	originalReadDir, originalRead := environmentReadDir, environmentReadJSONFile
	t.Cleanup(func() { environmentReadDir = originalReadDir; environmentReadJSONFile = originalRead })
	environmentReadDir = func(string) ([]os.DirEntry, error) { return []os.DirEntry{environmentInvalidDirEntry{}}, nil }
	environmentReadJSONFile = func(p string, required bool, value interface{}) error {
		if strings.HasSuffix(p, ".env.json") {
			return json.Unmarshal([]byte(`{"id":"dev"}`), value)
		}
		return originalRead(p, required, value)
	}
	if _, err := newFsEnvironmentsStore(root).loadEnvironments(context.Background()); err == nil || !strings.Contains(err.Error(), "invalid or duplicate live") {
		t.Fatalf("invalid Unicode directory ID accepted: %v", err)
	}
}

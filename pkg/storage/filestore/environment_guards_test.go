package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

func TestEnvironmentPathAndUninitializedGuards(t *testing.T) {
	root := environmentTestDir(t)
	f := &EnvironmentFiles{Root: root}
	for _, rel := range []string{"/absolute", "../escape", `environments\dev\dev.env.json`, "other", "environments/dev/other.json"} {
		if _, err := f.destination(rel); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
			t.Fatalf("accepted %q: %v", rel, err)
		}
	}
	if err := f.Commit(nil, "actor", dto.EnvironmentOperationResult{}); !os.IsPermission(err) {
		t.Fatalf("uninitialized commit: %v", err)
	}
	if _, err := f.Operation("op", "actor"); !os.IsNotExist(err) {
		t.Fatalf("uninitialized receipt: %v", err)
	}
	if err := WithEnvironmentFiles(context.Background(), "", "", nil); !errors.Is(err, datatug.ErrProjectDoesNotExist) {
		t.Fatal(err)
	}
	if err := withEnvironmentStoreLock(context.Background(), "", nil); !errors.Is(err, datatug.ErrProjectDoesNotExist) {
		t.Fatal(err)
	}
	if err := withEnvironmentStoreLock(context.Background(), filepath.Join(root, "missing"), nil); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := withEnvironmentStoreReadLock(context.Background(), filepath.Join(root, "missing"), nil); !os.IsNotExist(err) {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if err := environmentPlainPath(link); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, ".datatug-environments-owner.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := environmentNamespace(root, t.TempDir()); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
}
func TestEnvironmentCorruptPrivateEvidence(t *testing.T) {
	root := environmentTestDir(t)
	private := t.TempDir()
	if err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
		if err := os.WriteFile(filepath.Join(f.namespace, "untrusted"), []byte("private"), 0644); err != nil {
			return err
		}
		if _, err := readEnvironmentPrivateFile(filepath.Join(f.namespace, "untrusted")); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
			t.Fatalf("public private file accepted %v", err)
		}
		if err := f.CommitChecked(map[string][]byte{"datatug-project.json": []byte(`{}`)}, map[string]string{}, "actor", dto.EnvironmentOperationResult{}); err == nil {
			t.Fatal("missing fence accepted")
		}
		if err := f.CommitChecked(map[string][]byte{"datatug-project.json": []byte(`{}`)}, map[string]string{"datatug-project.json": "wrong"}, "actor", dto.EnvironmentOperationResult{}); err == nil {
			t.Fatal("wrong fence accepted")
		}
		if err := f.CommitChecked(map[string][]byte{"datatug-project.json": []byte(`{}`)}, map[string]string{"datatug-project.json": "absent"}, "actor", dto.EnvironmentOperationResult{OperationID: "op"}); err != nil {
			return err
		}
		file, err := f.LoadProjectFile()
		if err != nil {
			return err
		}
		if file.EnvironmentOrder != nil {
			t.Fatal("unexpected order")
		}
		receipt := filepath.Join(f.namespace, environmentHash([]byte("op"))+".receipt.json")
		if err := os.WriteFile(receipt, []byte("corrupt"), 0600); err != nil {
			return err
		}
		if _, err := f.Operation("op", "actor"); err == nil {
			t.Fatal("corrupt receipt accepted")
		}
		if err := os.WriteFile(filepath.Join(f.namespace, "intent.json"), []byte("corrupt"), 0600); err != nil {
			return err
		}
		if err := f.recover(); err == nil {
			t.Fatal("corrupt intent accepted")
		}
		return os.Remove(filepath.Join(f.namespace, "intent.json"))
	}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".datatug-environments.lock")
	if err := os.WriteFile(marker, []byte("other-owner"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := registerEnvironmentNamespace(root, "digest", "id"); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
	if _, err := environmentNamespace(root, private); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
}
func TestEnvironmentRecoveryRejectsTamperedTargets(t *testing.T) {
	for _, kind := range []string{"path", "stage", "changed", "corrupt-stage"} {
		t.Run(kind, func(t *testing.T) {
			root := environmentTestDir(t)
			private := t.TempDir()
			f := &EnvironmentFiles{Root: root, namespace: private}
			dest := filepath.Join(root, "datatug-project.json")
			old := []byte(`{"old":true}`)
			next := []byte(`{"new":true}`)
			if err := os.WriteFile(dest, old, 0600); err != nil {
				t.Fatal(err)
			}
			stage := filepath.Join(root, ".environment-stage-test")
			if err := os.WriteFile(stage, next, 0600); err != nil {
				t.Fatal(err)
			}
			replacement := environmentReplacement{Path: "datatug-project.json", Stage: stage, OldHash: environmentHash(old), NewHash: environmentHash(next)}
			switch kind {
			case "path":
				replacement.Path = "../escape"
			case "stage":
				replacement.Stage = filepath.Join(root, "untrusted")
			case "changed":
				if err := os.WriteFile(dest, []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
			case "corrupt-stage":
				if err := os.WriteFile(stage, []byte("corrupt"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			intent := environmentIntent{Files: []environmentReplacement{replacement}, Result: dto.EnvironmentOperationResult{ReplayUntil: time.Now().Format(time.RFC3339Nano)}}
			body, err := json.Marshal(intent)
			if err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(private, "intent.json"), body, 0600); err != nil {
				t.Fatal(err)
			}
			if err = os.WriteFile(filepath.Join(private, "commit"), []byte(environmentHash(body)), 0600); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(dest)
			if err != nil {
				t.Fatal(err)
			}
			if err = f.recover(); err == nil {
				t.Fatal("tampered transaction accepted")
			}
			after, err := os.ReadFile(dest)
			if err != nil || string(before) != string(after) {
				t.Fatalf("rejected transaction changed destination: %s %v", after, err)
			}
		})
	}
}

type environmentForeignInfo struct{ os.FileInfo }

func (f environmentForeignInfo) Sys() any { return nil }
func TestEnvironmentOwnershipGuards(t *testing.T) {
	for _, kind := range []string{"mode", "identity", "marker", "owner"} {
		t.Run(kind, func(t *testing.T) {
			root := environmentTestDir(t)
			private := t.TempDir()
			var namespace string
			if err := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error { namespace = f.namespace; return nil }); err != nil {
				t.Fatal(err)
			}
			locator := filepath.Join(root, ".datatug-environments-owner.json")
			switch kind {
			case "mode":
				if err := os.Chmod(locator, 0644); err != nil {
					t.Fatal(err)
				}
			case "identity":
				if err := os.WriteFile(filepath.Join(namespace, "namespace"), []byte("other"), 0600); err != nil {
					t.Fatal(err)
				}
			case "marker":
				if err := os.WriteFile(filepath.Join(root, ".datatug-environments.lock"), []byte("other"), 0600); err != nil {
					t.Fatal(err)
				}
			case "owner":
				original := environmentOS.Lstat
				t.Cleanup(func() { environmentOS.Lstat = original })
				environmentOS.Lstat = func(p string) (os.FileInfo, error) {
					st, err := original(p)
					if err == nil && p == locator {
						return environmentForeignInfo{st}, nil
					}
					return st, err
				}
			}
			if _, err := environmentNamespace(root, private); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
				t.Fatalf("unsafe ownership accepted: %v", err)
			}
		})
	}
	root := environmentTestDir(t)
	if err := os.Mkdir(filepath.Join(root, ".datatug-environments.lock"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ReadEnvironmentFiles(context.Background(), root, func(*EnvironmentFiles) error { t.Fatal("unsafe lock callback"); return nil }); !errors.Is(err, ErrEnvironmentRecoveryRequired) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ReadEnvironmentFiles(ctx, t.TempDir(), func(*EnvironmentFiles) error { t.Fatal("canceled callback"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	original := environmentOS.UserCacheDir
	t.Cleanup(func() { environmentOS.UserCacheDir = original })
	environmentOS.UserCacheDir = func() (string, error) { return "", errEnvironmentDisk }
	root = environmentTestDir(t)
	if err := os.WriteFile(filepath.Join(root, ".datatug-environments.lock"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := environmentNamespace(root, ""); !errors.Is(err, errEnvironmentDisk) {
		t.Fatal(err)
	}
}

func environmentTestDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

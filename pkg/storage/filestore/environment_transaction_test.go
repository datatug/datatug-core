package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/dto"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestEnvironmentTransactionChild(t *testing.T) {
	root := os.Getenv("DATATUG_TX_CHILD_ROOT")
	if root == "" {
		return
	}
	environmentTransactionFault = func(step string) {
		if step == os.Getenv("DATATUG_TX_CRASH") {
			os.Exit(77)
		}
	}
	err := WithEnvironmentFiles(context.Background(), root, os.Getenv("DATATUG_TX_CHILD_PRIVATE"), func(f *EnvironmentFiles) error {
		return f.Commit(map[string][]byte{"datatug-project.json": []byte(`{"environmentOrder":["new"]}`), "environments/new/new.env.json": []byte(`{"id":"new","dbServers":[]}`)}, "actor-A-authority-A", dto.EnvironmentOperationResult{OperationID: "operation-A", EnvironmentID: "new", Status: "pending"})
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestEnvironmentTransactionTwoProcessSharedRecovery(t *testing.T) {
	for _, point := range []string{"stages", "intent", "decision", "rename-0", "rename-1", "before-receipt"} {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			privateA := t.TempDir()
			privateB := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "datatug-project.json"), []byte(`{"environmentOrder":[]}`), 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestEnvironmentTransactionChild$")
			cmd.Env = append(os.Environ(), "DATATUG_TX_CHILD_ROOT="+root, "DATATUG_TX_CHILD_PRIVATE="+privateA, "DATATUG_TX_CRASH="+point)
			if err := cmd.Run(); err == nil {
				t.Fatal("child did not crash")
			}
			err := WithEnvironmentFiles(context.Background(), root, privateB, func(f *EnvironmentFiles) error {
				body, e := os.ReadFile(filepath.Join(root, "datatug-project.json"))
				if e != nil {
					return e
				}
				_, envErr := os.Stat(filepath.Join(root, "environments/new/new.env.json"))
				committed := point != "stages" && point != "intent"
				if committed {
					if string(body) != `{"environmentOrder":["new"]}` || envErr != nil {
						t.Fatal("partial recovered state")
					}
					if _, e = f.Operation("operation-A", "actor-B-authority-B"); !os.IsPermission(e) {
						t.Fatalf("other actor receipt %v", e)
					}
					r, e := f.Operation("operation-A", "actor-A-authority-A")
					if e != nil || r.EnvironmentID != "new" || r.Status != "committed" {
						t.Fatalf("lost accepted ID: %v %v", r, e)
					}
				} else if string(body) != `{"environmentOrder":[]}` || !os.IsNotExist(envErr) {
					t.Fatal("predecision changed canonical files")
				}
				return f.Commit(map[string][]byte{"datatug-project.json": []byte(`{"environmentOrder":[]}`)}, "actor-B-authority-B", dto.EnvironmentOperationResult{OperationID: "operation-B"})
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestEnvironmentRecoveryLocatorFailsClosed(t *testing.T) {
	for _, kind := range []string{"corrupt", "missing-locator", "missing-namespace", "private-mode"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			if err := WithEnvironmentFiles(context.Background(), root, t.TempDir(), func(*EnvironmentFiles) error { return nil }); err != nil {
				t.Fatal(err)
			}
			p := filepath.Join(root, ".datatug-environments-owner.json")
			b, _ := os.ReadFile(p)
			var locator environmentLocator
			_ = json.Unmarshal(b, &locator)
			switch kind {
			case "missing-locator":
				_ = os.Remove(p)
			case "corrupt":
				_ = os.WriteFile(p, []byte("broken"), 0600)
			case "missing-namespace":
				_ = os.RemoveAll(locator.Directory)
			case "private-mode":
				_ = os.Chmod(locator.Directory, 0755)
			}
			called := false
			err := WithEnvironmentFiles(context.Background(), root, t.TempDir(), func(*EnvironmentFiles) error { called = true; return nil })
			if err == nil || called {
				t.Fatalf("reinitialized inaccessible recovery: %v", err)
			}
		})
	}
}

func TestEnvironmentPrivateEvidenceExcludedFromPublicReads(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	private := t.TempDir()
	const secret = "private-operation-evidence-marker"
	if e := WithEnvironmentFiles(ctx, root, private, func(f *EnvironmentFiles) error {
		return f.Commit(map[string][]byte{"datatug-project.json": []byte(`{"id":"p","title":"Safe project","access":"private"}`), "environments/dev/dev.env.json": []byte(`{"id":"dev","title":"Development","dbServers":[]}`)}, "private-actor-binding", dto.EnvironmentOperationResult{OperationID: secret, PayloadDigest: secret})
	}); e != nil {
		t.Fatal(e)
	}
	store := newStore("files", map[string]string{"p": root})
	briefs, e := store.GetProjects(ctx)
	if e != nil {
		t.Fatal(e)
	}
	project, e := store.GetProjectStore("p").LoadProjectFile(ctx)
	if e != nil {
		t.Fatal(e)
	}
	envs, e := store.GetProjectStore("p").LoadEnvironments(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range []any{briefs, project, envs} {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		for _, forbidden := range []string{private, secret, "namespaceId", ".datatug-environments-owner", "private-actor-binding", "rootDigest"} {
			if bytes.Contains(b, []byte(forbidden)) {
				t.Fatalf("private environment evidence exported: %s", forbidden)
			}
		}
	}
	locator, e := os.ReadFile(filepath.Join(root, ".datatug-environments-owner.json"))
	if e != nil || !bytes.Contains(locator, []byte("namespaceId")) {
		t.Fatal("test did not create actual locator")
	}
}

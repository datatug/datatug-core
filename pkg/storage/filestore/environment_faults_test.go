package filestore

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/dto"
)

var errEnvironmentDisk = errors.New("injected environment disk failure")

type environmentFaultFile struct {
	environmentFile
	boundary func(string) bool
}

func (f environmentFaultFile) Chmod(m os.FileMode) error {
	if f.boundary("chmod") {
		return errEnvironmentDisk
	}
	return f.environmentFile.Chmod(m)
}
func (f environmentFaultFile) Write(b []byte) (int, error) {
	if f.boundary("write") {
		return 0, errEnvironmentDisk
	}
	return f.environmentFile.Write(b)
}
func (f environmentFaultFile) WriteAt(b []byte, o int64) (int, error) {
	if f.boundary("write-at") {
		return 0, errEnvironmentDisk
	}
	return f.environmentFile.WriteAt(b, o)
}
func (f environmentFaultFile) Sync() error {
	if f.boundary("sync") {
		return errEnvironmentDisk
	}
	return f.environmentFile.Sync()
}
func (f environmentFaultFile) Close() error {
	err := f.environmentFile.Close()
	if f.boundary("close") {
		return errors.Join(err, errEnvironmentDisk)
	}
	return err
}

// Each run fails exactly one real filesystem boundary. Files still use real
// inodes and bytes; after removing the fault the same owner must recover either
// the complete old state or the complete committed state.
func TestEnvironmentTransactionDiskFailureMatrix(t *testing.T) {
	for _, gitProject := range []bool{false, true} {
		name := "external"
		if gitProject {
			name = "git"
		}
		t.Run(name, func(t *testing.T) {
			baseline := runEnvironmentDiskScenario(t, gitProject, 0)
			for at := 1; at <= baseline; at++ {
				t.Run(itoaEnvironment(at), func(t *testing.T) { runEnvironmentDiskScenario(t, gitProject, at) })
			}
		})
	}
}
func itoaEnvironment(v int) string { return time.Unix(int64(v), 0).UTC().Format("150405") }
func runEnvironmentDiskScenario(t *testing.T, gitProject bool, failAt int) int {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	private := t.TempDir()
	if gitProject {
		cmd := exec.Command("git", "-C", root, "init", "--quiet")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %s %v", output, err)
		}
	}
	project := filepath.Join(root, "datatug-project.json")
	old := []byte(`{"environmentOrder":[]}`)
	if err := os.WriteFile(project, old, 0600); err != nil {
		t.Fatal(err)
	}
	original := environmentOS
	calls := 0
	boundary := func(_ string) bool { calls++; return failAt > 0 && calls == failAt }
	value := reflect.ValueOf(&environmentOS).Elem()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		real := reflect.ValueOf(original).Field(i)
		kind := value.Type().Field(i).Name
		field.Set(reflect.MakeFunc(field.Type(), func(args []reflect.Value) []reflect.Value {
			if boundary(kind) {
				out := make([]reflect.Value, real.Type().NumOut())
				for j := range out {
					out[j] = reflect.Zero(real.Type().Out(j))
				}
				out[len(out)-1] = reflect.ValueOf(errEnvironmentDisk)
				return out
			}
			out := real.Call(args)
			if len(out) == 2 && out[1].IsNil() {
				if f, ok := out[0].Interface().(environmentFile); ok {
					out[0] = reflect.ValueOf(environmentFaultFile{f, boundary})
				}
			}
			return out
		}))
	}
	t.Cleanup(func() { environmentOS = original })
	var namespace string
	result := dto.EnvironmentOperationResult{OperationID: "operation", ReplayUntil: time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)}
	operationErr := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error {
		namespace = f.namespace
		if err := f.Commit(map[string][]byte{"datatug-project.json": []byte(`{"environmentOrder":["dev"]}`)}, "actor", result); err != nil {
			return err
		}
		receipt, err := f.Operation("operation", "actor")
		if err != nil {
			return err
		}
		if receipt.Status != "expired" {
			t.Fatalf("expiry not persisted: %+v", receipt)
		}
		return nil
	})
	if operationErr == nil {
		operationErr = ReadEnvironmentFiles(context.Background(), root, func(f *EnvironmentFiles) error {
			return nil
		})
	}
	total := calls
	environmentOS = original
	if failAt == 0 && operationErr != nil {
		t.Fatal(operationErr)
	}
	// A probe may legitimately tolerate a failed existence check. Whenever a
	// failure was returned it must retain its cause or fail closed for recovery.
	if operationErr != nil && !errors.Is(operationErr, errEnvironmentDisk) && !errors.Is(operationErr, ErrEnvironmentRecoveryRequired) {
		t.Fatalf("lost disk failure: %v", operationErr)
	}
	// Initialization failures before the locator is durable may leave an orphaned
	// private directory, but must never alter the canonical project file.
	body, err := os.ReadFile(project)
	if err != nil {
		t.Fatal(err)
	}
	if namespace == "" {
		if string(body) != string(old) {
			t.Fatalf("initialization changed project: %s", body)
		}
		return total
	}
	recoveryErr := WithEnvironmentFiles(context.Background(), root, private, func(f *EnvironmentFiles) error { return nil })
	if recoveryErr != nil {
		t.Fatalf("same-owner recovery failed after boundary %d: %v", failAt, recoveryErr)
	}
	body, err = os.ReadFile(project)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != string(old) && string(body) != `{"environmentOrder":["dev"]}` {
		t.Fatalf("partial canonical state: %s", body)
	}
	return total
}

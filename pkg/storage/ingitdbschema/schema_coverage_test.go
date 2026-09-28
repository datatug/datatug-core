package ingitdbschema

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

type subErrorFS struct{}

func (subErrorFS) Sub(dir string) (fs.FS, error) {
	return nil, errors.New("sub failed")
}

func (subErrorFS) Open(name string) (fs.File, error) {
	return nil, errors.New("open failed")
}

func (subErrorFS) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("read failed")
}

type walkErrorFS struct{}

func (walkErrorFS) Open(name string) (fs.File, error) {
	return nil, errors.New("walk error")
}

type subWalkErrorFS struct{}

func (subWalkErrorFS) Sub(dir string) (fs.FS, error) {
	return walkErrorFS{}, nil
}

func (subWalkErrorFS) Open(name string) (fs.File, error) {
	return nil, errors.New("open failed")
}

func (subWalkErrorFS) ReadFile(name string) ([]byte, error) {
	return nil, errors.New("read failed")
}

type mockTempFile struct {
	path     string
	writeErr error
	chmodErr error
	syncErr  error
	closeErr error
}

func (m *mockTempFile) Write(p []byte) (n int, err error) {
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}

func (m *mockTempFile) Chmod(mode os.FileMode) error {
	return m.chmodErr
}

func (m *mockTempFile) Sync() error {
	return m.syncErr
}

func (m *mockTempFile) Close() error {
	return m.closeErr
}

func (m *mockTempFile) Name() string {
	return m.path
}

func TestWriteSchema_SubAndWalkErrors(t *testing.T) {
	oldFS := schemaFS
	defer func() { schemaFS = oldFS }()

	t.Run("sub_error", func(t *testing.T) {
		schemaFS = subErrorFS{}
		err := WriteSchema(t.TempDir())
		if err == nil {
			t.Fatal("expected error from broken SubFS")
		}
	})

	t.Run("walk_error", func(t *testing.T) {
		schemaFS = subWalkErrorFS{}
		err := WriteSchema(t.TempDir())
		if err == nil {
			t.Fatal("expected error from broken WalkDir")
		}
	})
}

func TestCreateFile_TempFileErrors(t *testing.T) {
	oldCreateTemp := osCreateTemp
	defer func() { osCreateTemp = oldCreateTemp }()

	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")

	t.Run("write_error", func(t *testing.T) {
		tmpPath := filepath.Join(dir, "temp-write")
		_ = os.WriteFile(tmpPath, []byte("x"), 0o644)
		osCreateTemp = func(dir, pattern string) (tempFile, error) {
			return &mockTempFile{path: tmpPath, writeErr: errors.New("write fail")}, nil
		}
		err := createFile(target, []byte("data"))
		if err == nil {
			t.Fatal("expected write error")
		}
	})

	t.Run("chmod_error", func(t *testing.T) {
		tmpPath := filepath.Join(dir, "temp-chmod")
		_ = os.WriteFile(tmpPath, []byte("x"), 0o644)
		osCreateTemp = func(dir, pattern string) (tempFile, error) {
			return &mockTempFile{path: tmpPath, chmodErr: errors.New("chmod fail")}, nil
		}
		err := createFile(target, []byte("data"))
		if err == nil {
			t.Fatal("expected chmod error")
		}
	})

	t.Run("sync_error", func(t *testing.T) {
		tmpPath := filepath.Join(dir, "temp-sync")
		_ = os.WriteFile(tmpPath, []byte("x"), 0o644)
		osCreateTemp = func(dir, pattern string) (tempFile, error) {
			return &mockTempFile{path: tmpPath, syncErr: errors.New("sync fail")}, nil
		}
		err := createFile(target, []byte("data"))
		if err == nil {
			t.Fatal("expected sync error")
		}
	})

	t.Run("close_error", func(t *testing.T) {
		tmpPath := filepath.Join(dir, "temp-close")
		_ = os.WriteFile(tmpPath, []byte("x"), 0o644)
		osCreateTemp = func(dir, pattern string) (tempFile, error) {
			return &mockTempFile{path: tmpPath, closeErr: errors.New("close fail")}, nil
		}
		err := createFile(target, []byte("data"))
		if err == nil {
			t.Fatal("expected close error")
		}
	})

	t.Run("rename_error", func(t *testing.T) {
		osCreateTemp = oldCreateTemp
		dirTarget := filepath.Join(dir, "isadir")
		_ = os.MkdirAll(dirTarget, 0o755)
		err := createFile(dirTarget, []byte("data"))
		if err == nil {
			t.Fatal("expected rename error")
		}
	})
}

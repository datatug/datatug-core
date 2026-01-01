package filestore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/datatug/datatug-core/pkg/storage/dtprojcreator"
)

func NewStorage(projPath string) dtprojcreator.Storage {
	return fsStorage{
		projPath: projPath,
	}
}

type fsStorage struct {
	projPath string
}

func (f fsStorage) Commit(_ context.Context, _ string) error {
	return nil //No commit required as all files are changes are applied directly to the file system
}

func (f fsStorage) FileExists(_ context.Context, filePath string) (bool, error) {
	filePath = path.Join(f.projPath, filePath)
	_, err := osStat(filePath)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func (f fsStorage) OpenFile(_ context.Context, filePath string) (io.ReadCloser, error) {
	filePath = path.Join(f.projPath, filePath)
	return osOpen(filePath)
}

func (f fsStorage) WriteFile(_ context.Context, filePath string, reader io.Reader) error {
	filePath = path.Join(f.projPath, filePath)
	dir := filepath.Dir(filePath)
	if err := osMkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	file, err := osCreate(filePath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}
	return nil
}

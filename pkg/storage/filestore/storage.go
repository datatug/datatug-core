package filestore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"

	"github.com/datatug/datatug-core/internal/plainfs"
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
	// The folders and the file are made only through plain files and plain
	// folders of the project: a link, or a path that leaves the project, is
	// refused (see internal/plainfs).
	if err := plainfs.WriteFile(f.projPath, path.Join(f.projPath, filePath), func(w io.Writer) error {
		if _, err := io.Copy(w, reader); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	return nil
}

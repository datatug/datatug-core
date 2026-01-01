package dtprojcreator

import (
	"context"
	"io"
)

type Storage interface {
	FileExists(ctx context.Context, filePath string) (bool, error)
	OpenFile(ctx context.Context, filePath string) (io.ReadCloser, error)
	WriteFile(ctx context.Context, filePath string, reader io.Reader) error
	Commit(ctx context.Context, message string) error
}

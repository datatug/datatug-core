package filestore

import (
	"fmt"
	"io"
	"path"

	"github.com/datatug/datatug-core/internal/plainfs"
)

// saveReadme writes dirPath/README.md inside projectDir through plain files
// and plain folders only (see internal/plainfs).
func saveReadme(projectDir, dirPath string, saver func(w io.Writer) error) (err error) {
	f, err := plainfs.CreateFile(projectDir, path.Join(dirPath, "README.md"))
	if err != nil {
		return fmt.Errorf("failed to created README.md for DB server: %w", err)
	}
	defer func() {
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}()
	return saver(f)
}

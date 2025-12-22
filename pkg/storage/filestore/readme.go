package filestore

import (
	"fmt"
	"io"
	"os"
	"path"
)

func saveReadme(dirPath string, saver func(w io.Writer) error) error {
	filePath := path.Join(dirPath, "README.md")
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to created README.md for DB server: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()
	return saver(f)
}

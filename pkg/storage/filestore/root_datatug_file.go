package filestore

import (
	"fmt"
	"io"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"gopkg.in/yaml.v3"
)

func LoadRootDatatugFile(dir string) (repoRootFile *datatug.RepoRootFile, err error) {
	var f io.ReadCloser
	filePath := path.Join(dir, RepoRootDataTugFileName)
	if f, err = osOpen(filePath); err != nil {
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			if err == nil {
				err = fmt.Errorf("failed to close repository's root .datatug.yaml file opened for read: %v", closeErr)
			}
		}
	}()
	decoder := yaml.NewDecoder(f)
	if err = decoder.Decode(&repoRootFile); err != nil {
		err = fmt.Errorf("failed to decode .datatug.yaml file: %w", err)
		return
	}
	return
}

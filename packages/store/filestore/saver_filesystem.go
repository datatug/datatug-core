package filestore

import (
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/parallel"
	"log"
	"os"
	"path"
)

// fileSystemSaver saves or updates DataTug project
type fileSystemSaver struct {
	// pathByID map[string]string
	projDirPath   string
	readmeEncoder models.ReadmeEncoder
}

// newSaver creates a new project saver
func newSaver(projDirPath string, readmeEncoder models.ReadmeEncoder) fileSystemSaver {
	return fileSystemSaver{
		projDirPath:   projDirPath,
		readmeEncoder: readmeEncoder,
	}
}

func (s fileSystemSaver) saveJSONFile(dirPath, fileName string, v interface{}) (err error) {
	if err = os.MkdirAll(dirPath, os.ModeDir); err != nil {
		return fmt.Errorf("failed to create boards folder: %w", err)
	}

	fullFileName := path.Join(dirPath, fileName)
	log.Printf("Saving file: %v\n%+v", fullFileName, v)
	file, _ := os.OpenFile(fullFileName, os.O_CREATE, os.ModePerm)
	defer func() {
		_ = file.Close()
	}()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "\t")
	if err = encoder.Encode(v); err != nil {
		return err
	}
	return err
}

// Saves each item in a parallel
func (s fileSystemSaver) saveItems(plural string, count int, getWorker func(i int) func() error) error {
	log.Printf("Saving %v %v...", count, plural)
	switch count {
	case 0:
		log.Printf("No " + plural)
		return nil
	case 1:
		return getWorker(0)()
	}
	workers := make([]func() error, count)
	for i := 0; i < count; i++ {
		workers[i] = getWorker(i)
	}
	if err := parallel.Run(workers...); err != nil {
		return fmt.Errorf("failed to save %v: %w", plural, err)
	}
	return nil
}

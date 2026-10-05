package filestore

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"path"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/parallel"
)

//// fileSystemSaver saves or updates DataTug project
//type fileSystemSaver struct {
//	// pathByID map[string]string
//	projFileMutex *sync.Mutex
//	projDirPath   string
//	readmeEncoder models.ReadmeEncoder
//}
//
//// newSaver creates a new project saver
//func newSaver(projDirPath string, readmeEncoder models.ReadmeEncoder) fileSystemSaver {
//	return fileSystemSaver{
//		projDirPath:   projDirPath,
//		readmeEncoder: readmeEncoder,
//		projFileMutex: new(sync.Mutex),
//	}
//}

// saveJSONFile writes v as indented JSON to dirPath/fileName, making the
// folders above it. Both must be inside projectDir, and the write goes only
// through plain files and plain folders: a link, in the file's place or in a
// folder above it, is refused (see internal/plainfs). It returns the error of
// the create, of the encode and of the close.
func saveJSONFile(projectDir, dirPath, fileName string, v interface{ Validate() error }) error {
	if err := v.Validate(); err != nil {
		return fmt.Errorf("an attempt to save invalid data %T: %w", v, err)
	}
	return plainfs.WriteFile(projectDir, path.Join(dirPath, fileName), func(w io.Writer) error {
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "\t")
		return encoder.Encode(v)
	})
}

// Saves each item in a parallel
func saveItems(plural string, count int, getWorker func(i int) func() error) error {
	//log.Printf("Saving %v %v...", count, plural)
	switch count {
	case 0:
		log.Print("No " + plural)
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

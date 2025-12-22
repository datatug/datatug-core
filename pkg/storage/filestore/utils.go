package filestore

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// Decoder decodes
type Decoder interface {
	Decode(o interface{}) error
}

func readJSONFile(filePath string, required bool, o interface{}) (err error) {
	jsonDecoderFactory := func(r io.Reader) Decoder {
		return json.NewDecoder(r)
	}
	return readFile(filePath, required, o, jsonDecoderFactory)
}

func readFile(filePath string, required bool, o interface{}, newDecoder func(r io.Reader) Decoder) (err error) {
	var file *os.File
	if file, err = os.Open(filePath); err != nil {
		if os.IsNotExist(err) && !required {
			err = nil
		}
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			log.Printf("failed to close file %v: %v", filePath, err)
		}
	}()
	decoder := newDecoder(file)
	if err = decoder.Decode(o); err != nil {
		return err
	}
	return err
}

func DirExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err // some other error
	}
	return info.IsDir(), nil
}

// ExpandHome expands leading ~ to the user's home directory.
func ExpandHome(p string) string {
	if p == "" {
		return p
	}
	if strings.HasPrefix(p, "~/") || p == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if p == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(p, "~/"))
		}
	}
	return p
}

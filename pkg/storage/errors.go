package storage

import (
	"fmt"
	"strings"
)

func NewFileLoadError(fileName string, err error) FileLoadError {
	return FileLoadError{
		FileName: fileName,
		err:      err,
		Err:      err.Error(),
	}
}

type FileLoadError struct {
	FileName string `json:"fileName,omitempty"`
	Err      string `json:"err,omitempty"`
	err      error
}

func (e FileLoadError) Error() string {
	return fmt.Sprintf("%v: %v", e.FileName, e.err)
}

func (e FileLoadError) String() string {
	return e.Error()
}

func NewFilesLoadError(errs []FileLoadError) FilesLoadError {
	return FilesLoadError{
		errs: errs,
	}
}

type FilesLoadError struct {
	errs []FileLoadError
}

func (e FilesLoadError) Errors() []FileLoadError {
	var errs []FileLoadError
	errs = append(errs, e.errs...)
	return errs
}

func (e FilesLoadError) Error() string {
	if len(e.errs) == 1 {
		return fmt.Sprintf("1 file failed to load: %s", e.errs[0].Error())
	}
	errs := make([]string, len(e.errs))
	for i, err := range e.errs {
		errs[i] = err.Error()
	}
	return fmt.Sprintf("%d files failed to load:\n\t%s", len(e.errs), strings.Join(errs, "\n\t"))
}

func (e FilesLoadError) String() string {
	return e.Error()
}

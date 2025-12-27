package storage

import "fmt"

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
	return fmt.Sprintf("%d files failed to load", len(e.errs))
}

func (e FilesLoadError) String() string {
	return e.Error()
}

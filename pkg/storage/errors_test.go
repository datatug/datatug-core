package storage

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFileLoadError(t *testing.T) {
	errText := "some error"
	fileName := "test.json"
	err := errors.New(errText)
	fle := NewFileLoadError(fileName, err)

	assert.Equal(t, fileName, fle.FileName)
	assert.Equal(t, errText, fle.Err)
	assert.Equal(t, fileName+": "+errText, fle.Error())
	assert.Equal(t, fileName+": "+errText, fle.String())
}

func TestFilesLoadError(t *testing.T) {
	err1 := NewFileLoadError("file1", errors.New("err1"))
	err2 := NewFileLoadError("file2", errors.New("err2"))
	errs := []FileLoadError{err1, err2}

	err := NewFilesLoadError(errs)

	assert.Equal(t, errs, err.Errors())
	assert.Equal(t, "2 files failed to load:\n\tfile1: err1\n\tfile2: err2", err.Error())
	assert.Equal(t, err.Error(), err.String())

	singleErr := NewFilesLoadError([]FileLoadError{err1})
	assert.Equal(t, "1 file failed to load: file1: err1", singleErr.Error())
}

func TestFilesLoadError_Unwrap(t *testing.T) {
	cause := errors.New("cause")
	fle := NewFileLoadError("file1", cause)
	if !errors.Is(fle, cause) {
		t.Error("expected errors.Is to see through a FileLoadError")
	}
	all := NewFilesLoadError([]FileLoadError{NewFileLoadError("file0", errors.New("other")), fle})
	if !errors.Is(all, cause) {
		t.Error("expected errors.Is to see through a FilesLoadError to any file's error")
	}
	var target FileLoadError
	if !errors.As(all, &target) || target.FileName != "file0" {
		t.Errorf("expected errors.As to find the first FileLoadError, got %+v", target)
	}
}

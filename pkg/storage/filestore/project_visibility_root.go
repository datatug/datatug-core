package filestore

import (
	"errors"
	"io"
	"io/fs"
	"os"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// ReadProjectArchivedItemsFromRoot reads visibility through the caller's opened
// project root and binds the opened archive to the inspected regular file.
// It never reopens root.Name() or closes the caller's root. The result is display
// metadata only; it conveys no authorization or execution restriction.
func ReadProjectArchivedItemsFromRoot(root *os.Root) (map[datatug.ProjectObjectRef]bool, error) {
	if root == nil {
		return nil, datatug.ErrProjectUsageIncomplete
	}
	return projectArchivedItems(readProjectVisibilityRoot(root))
}

type visibilityRootReader interface {
	Lstat(string) (fs.FileInfo, error)
	OpenFile(string, int, fs.FileMode) (*os.File, error)
}

func readProjectVisibilityRoot(root visibilityRootReader) (projectVisibility, error) {
	state := projectVisibility{Version: 1}
	info, err := root.Lstat(visibilityFile)
	if errors.Is(err, fs.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if !info.Mode().IsRegular() || info.Size() > 2<<20 {
		return state, datatug.ErrProjectUsageIncomplete
	}
	file, err := root.OpenFile(visibilityFile, os.O_RDONLY|openNoFollowFlags, 0)
	if err != nil {
		return state, err
	}
	defer func() { _ = file.Close() }()
	opened, err := file.Stat()
	if err != nil {
		return state, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return state, datatug.ErrProjectUsageIncomplete
	}
	body, err := io.ReadAll(io.LimitReader(file, (2<<20)+1))
	if err != nil {
		return state, err
	}
	if len(body) > 2<<20 {
		return state, datatug.ErrProjectUsageIncomplete
	}
	return decodeProjectVisibility(body)
}

package filestore

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
)

var _ storage.FoldersStore = (*fsFoldersStore)(nil)

type fsFoldersStore struct {
	fsProjectStore
}

func (f fsFoldersStore) CreateFolder(_ context.Context, request storage.CreateFolderRequest) (folder *datatug.Folder, err error) {
	if err := datatug.ValidateFolderPath(request.Path); err != nil {
		return nil, validation.NewErrBadRequestFieldValue("path", err.Error())
	}
	panic("implement me")
}

func (f fsFoldersStore) GetFolder(_ context.Context, path string) (folder *datatug.Folder, err error) {
	panic("implement me: path=" + path)
}

func (f fsFoldersStore) DeleteFolder(_ context.Context, path string) (err error) {
	panic("implement me: path=" + path)
}

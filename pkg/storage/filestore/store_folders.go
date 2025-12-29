package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
)

var _ datatug.FoldersStore = (*fsFoldersStore)(nil)

const FoldersDir = "folders"

func newFsFoldersStore(projectPath string) fsFoldersStore {
	return fsFoldersStore{
		fsProjectItemsStore: newFsProjectItemsStore[datatug.Folders, *datatug.Folder, datatug.Folder](
			path.Join(projectPath, FoldersDir), "",
		),
	}
}

type fsFoldersStore struct {
	fsProjectItemsStore[datatug.Folders, *datatug.Folder, datatug.Folder]
}

func (s fsFoldersStore) LoadFolders(ctx context.Context, o ...datatug.StoreOption) (datatug.Folders, error) {
	return s.loadProjectItems(ctx, s.dirPath, o...)
}

func (s fsFoldersStore) LoadFolder(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Folder, error) {
	dirPath := path.Join(s.dirPath, id)
	return s.loadProjectItem(ctx, dirPath, id, "", o...)
}

func (s fsFoldersStore) SaveFolder(ctx context.Context, folderPath string, folder *datatug.Folder) error {
	dirPath := path.Join(s.dirPath, folderPath)
	return s.saveProjectItem(ctx, dirPath, folder)
}

func (s fsFoldersStore) SaveFolders(ctx context.Context, folderPath string, folders datatug.Folders) error {
	dirPath := path.Join(s.dirPath, folderPath)
	return s.saveProjectItems(ctx, dirPath, folders)
}

func (s fsFoldersStore) CreateFolder(_ context.Context, request storage.CreateFolderRequest) (folder *datatug.Folder, err error) {
	if err = datatug.ValidateFolderPath(request.Path); err != nil {
		return nil, validation.NewErrBadRequestFieldValue("path", err.Error())
	}
	panic("implement me")
}

func (s fsFoldersStore) DeleteFolder(ctx context.Context, folderDir, id string) (err error) {
	dirPath := path.Join(s.dirPath, folderDir)
	return s.deleteProjectItem(ctx, dirPath, id)
}

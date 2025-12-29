package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
)

var _ datatug.FoldersStore = (*fsFoldersStore)(nil)

const FoldersDir = "folders"

func newFsFoldersStore(projectPath string) fsFoldersStore {
	return fsFoldersStore{
		fsProjectItemsStore: newDirProjectItemsStore[datatug.Folders, *datatug.Folder, datatug.Folder](
			path.Join(projectPath, FoldersDir), ".datatug-folder.json",
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
	return s.loadProjectItem(ctx, s.dirPath, id, "", o...)
}

func (s fsFoldersStore) SaveFolder(ctx context.Context, folderPath string, folder *datatug.Folder) error {
	dirPath := path.Join(s.dirPath, folderPath)
	return s.saveProjectItem(ctx, dirPath, folder)
}

func (s fsFoldersStore) SaveFolders(ctx context.Context, folderPath string, folders datatug.Folders) error {
	dirPath := path.Join(s.dirPath, folderPath)
	return s.saveProjectItems(ctx, dirPath, folders)
}

func (s fsFoldersStore) DeleteFolder(ctx context.Context, id string) (err error) {
	folderPath := path.Base(id)
	dirPath := path.Join(s.dirPath, folderPath)
	return s.deleteProjectItem(ctx, dirPath, id)
}

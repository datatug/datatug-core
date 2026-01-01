package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.BoardsStore = (*fsBoardsStore)(nil)

func newFsBoardsStore(projectPath string) fsBoardsStore {
	return fsBoardsStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board](
			path.Join(projectPath, storage.BoardsFolder), storage.BoardFileSuffix,
		),
	}
}

type fsBoardsStore struct {
	fsProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board]
}

func (s fsBoardsStore) LoadBoards(ctx context.Context, o ...datatug.StoreOption) (datatug.Boards, error) {
	items, err := s.loadProjectItems(ctx, s.dirPath, o...)
	return items, err
}

func (s fsBoardsStore) LoadBoard(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Board, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, s.itemFileName(id), o...)
}

func (s fsBoardsStore) SaveBoard(ctx context.Context, board *datatug.Board) error {
	return s.saveProjectItem(ctx, s.dirPath, board)
}

func (s fsBoardsStore) saveBoards(ctx context.Context, boards datatug.Boards) error {
	return s.saveProjectItems(ctx, s.dirPath, boards)
}

func (s fsBoardsStore) DeleteBoard(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, s.dirPath, id)
}

package filestore

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

var _ datatug.BoardsStore = (*fsBoardsStore)(nil)

type fsBoardsStore struct {
	fsProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board]
}

func (s fsBoardsStore) LoadBoards(ctx context.Context, o ...datatug.StoreOption) (datatug.Boards, error) {
	items, err := s.loadProjectItems(ctx, o...)
	return items, err
}

func (s fsBoardsStore) LoadBoard(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.Board, error) {
	return s.loadProjectItem(ctx, id, s.itemFileName(id), o...)
}

func (s fsBoardsStore) SaveBoard(ctx context.Context, board *datatug.Board) error {
	return s.saveProjectItem(ctx, board)
}

func (s fsBoardsStore) saveBoards(ctx context.Context, boards datatug.Boards) error {
	return s.saveProjectItems(ctx, BoardsFolder, boards)
}

func (s fsBoardsStore) DeleteBoard(ctx context.Context, id string) error {
	return s.deleteProjectItem(ctx, id)
}

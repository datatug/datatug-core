package filestore

import (
	"context"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.BoardsStore = (*fsBoardsStore)(nil)

type fsBoardsStore struct {
	fsProjectStore
	boardsDirPath string
}

func (store fsBoardsStore) Project() storage.ProjectStore {
	return store.fsProjectStore
}

func (store fsBoardsStore) Board(id string) storage.BoardStore {
	return store.board(id)
}

func (store fsBoardsStore) board(id string) fsBoardStore {
	return newFsBoardStore(id, store)
}

func newFsBoardsStore(fsProjectStore fsProjectStore) fsBoardsStore {
	return fsBoardsStore{
		fsProjectStore: fsProjectStore,
		boardsDirPath:  path.Join(fsProjectStore.projectPath, BoardsFolder),
	}
}

func (store fsBoardsStore) saveBoards(ctx context.Context, boards models.Boards) (err error) {
	return saveItems(BoardsFolder, len(boards), func(i int) func() error {
		return func() error {
			board := boards[i]
			boardStore := store.board(board.ID)
			return boardStore.SaveBoard(ctx, *board)
		}
	})
}

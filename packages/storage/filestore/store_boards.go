package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"path"
)

var _ storage.BoardsStore = (*fsBoardsStore)(nil)
var _ storage.BoardsSaver = (*fsBoardsStore)(nil)
var _ storage.BoardsLoader = (*fsBoardsStore)(nil)

type fsBoardsStore struct {
	fsProjectStore
	boardsDirPath string
}

func (store fsBoardsStore) DeleteBoard(boardID string) (err error) {
	panic("implement me")
}

func (store fsBoardsStore) SaveBoard(board models.Board) (err error) {
	panic("implement me")
}

func (store fsBoardsStore) Loader() storage.BoardsLoader {
	return store
}

func (store fsBoardsStore) Saver() storage.BoardsSaver {
	return store
}

func newFsBoardsStore(fsProjectStore fsProjectStore) fsBoardsStore {
	return fsBoardsStore{
		fsProjectStore: fsProjectStore,
		boardsDirPath:  path.Join(fsProjectStore.projectPath, BoardsFolder),
	}
}

// LoadBoard loads board
func (store fsBoardsStore) LoadBoard(boardID string) (board models.Board, err error) {
	fileName := path.Join(store.boardsDirPath, fmt.Sprintf("%v.json", boardID))
	if err = readJSONFile(fileName, true, &board); err != nil {
		err = fmt.Errorf("faile to load board [%v] from project [%v]: %w", boardID, store.projectID, err)
		return
	}
	return
}

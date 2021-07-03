package filestore

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"os"
	"path"
)

var _ storage.BoardStore = (*fsBoardStore)(nil)

type fsBoardStore struct {
	boardID string
	fsBoardsStore
}

func newFsBoardStore(boardID string, fsBoardsStore fsBoardsStore) fsBoardStore {
	return fsBoardStore{
		boardID: boardID,
		fsBoardsStore: fsBoardsStore,
	}
}


func (store fsBoardStore) ID() string {
	return store.boardID
}

func (store fsBoardStore) Boards() storage.BoardsStore {
	return store.fsBoardsStore
}

func (store fsBoardStore) DeleteBoard() (err error) {
	filePath := path.Join(store.boardsDirPath, jsonFileName(store.boardID, boardFileSuffix))
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(filePath)
}

func (store fsBoardStore) SaveBoard(board models.Board) (err error) {
	if err = store.updateProjectFileWithBoard(board); err != nil {
		return fmt.Errorf("failed to update project file with board: %w", err)
	}
	fileName := jsonFileName(board.ID, boardFileSuffix)
	board.ID = ""
	if err = saveJSONFile(
		store.boardsDirPath,
		fileName,
		board,
	); err != nil {
		return fmt.Errorf("failed to save board file: %w", err)
	}
	return err
}

// LoadBoard loads board
func (store fsBoardStore) LoadBoard() (*models.Board, error) {
	fileName := path.Join(store.boardsDirPath, fmt.Sprintf("%v.json", store.boardID))
	var board models.Board
	if err := readJSONFile(fileName, true, &board); err != nil {
		err = fmt.Errorf("faile to load board [%v] from project [%v]: %w", store.boardID, store.projectID, err)
		return nil, err
	}
	return &board, nil
}

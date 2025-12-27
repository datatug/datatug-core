package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s fsProjectStore) saveBoards(ctx context.Context, boards datatug.Boards) error {
	return saveItems(BoardsFolder, len(boards), func(i int) func() error {
		return func() error {
			return s.saveBoard(ctx, boards[i])
		}
	})
}

func (s fsProjectStore) deleteBoard(_ context.Context, id string) (err error) {
	boardsDirPath := path.Join(s.projectPath, DatatugFolder, BoardsFolder)
	filePath := path.Join(boardsDirPath, jsonFileName(id, boardFileSuffix))
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(filePath)
}

func (s fsProjectStore) saveBoard(_ context.Context, board *datatug.Board) error {
	boardsDirPath := path.Join(s.projectPath, DatatugFolder, BoardsFolder)
	fileName := jsonFileName(board.ID, boardFileSuffix)
	board.ID = ""
	if err := saveJSONFile(
		boardsDirPath,
		fileName,
		board,
	); err != nil {
		return fmt.Errorf("failed to save board file: %w", err)
	}
	return nil
}

// loadBoard is called by LoadBoard, loadBoards
func (s fsProjectStore) loadBoard(_ context.Context, id, fileName string, _ ...datatug.StoreOption) (*datatug.Board, error) {
	filePath := path.Join(s.projectPath, DatatugFolder, BoardsFolder, fileName)
	var board datatug.Board
	if err := readJSONFile(filePath, true, &board); err != nil {
		return nil, fmt.Errorf("failed to load board [%v] from project [%v]: %w", id, s.projectID, err)
	}
	if board.ID == "" {
		board.ID = id
	}
	return &board, nil
}

// loadBoards is called by LoadBoards
func (s fsProjectStore) loadBoards(_ context.Context, _ ...datatug.StoreOption) (boards datatug.Boards, err error) {
	boardsDirPath := path.Join(s.projectPath, DatatugFolder, BoardsFolder)

	if err = loadDir(nil, boardsDirPath, "*.json", processFiles,
		func(files []os.FileInfo) {
			boards = make(datatug.Boards, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if f.IsDir() {
				return nil
			}
			boardID, suffix := getProjItemIDFromFileName(f.Name())
			if suffix != boardFileSuffix {
				return nil
			}
			board, err := s.loadBoard(nil, boardID, f.Name())
			if err != nil {
				return err
			}
			mutex.Lock()
			boards = append(boards, board)
			mutex.Unlock()
			return nil
		}); err != nil {
		return nil, err
	}
	return boards, nil
}

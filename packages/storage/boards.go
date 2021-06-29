package storage

import "github.com/datatug/datatug/packages/models"

// BoardsStore defines DAL for boards
type BoardsStore interface {
	Loader() BoardsLoader
	Saver() BoardsSaver
}

// BoardsLoader loads boards
type BoardsLoader interface {
	// LoadBoard loads board
	LoadBoard(boardID string) (board models.Board, err error)
}

// BoardsSaver saves boards
type BoardsSaver interface {
	DeleteBoard(boardID string) (err error)
	SaveBoard(board models.Board) (err error)
}

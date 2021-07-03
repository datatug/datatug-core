package storage

import "github.com/datatug/datatug/packages/models"

// BoardsStore provides access to board records
type BoardsStore interface {
	ProjectStoreRef
	Board(id string) BoardStore
}

// BoardStore provides access to board record
type BoardStore interface {
	ID() string
	Boards() BoardsStore
	// LoadBoard loads board
	LoadBoard() (board *models.Board, err error)
	DeleteBoard() (err error)
	SaveBoard(board models.Board) (err error)
}

package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

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
	LoadBoard(ctx context.Context) (board *models.Board, err error)
	DeleteBoard(ctx context.Context) (err error)
	SaveBoard(ctx context.Context, board models.Board) (err error)
}

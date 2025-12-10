package storage

import (
	"context"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// BoardsStore provides access to board records
type BoardsStore interface {
	ProjectStoreRef
	CreateBoard(ctx context.Context, board datatug.Board) (*datatug.Board, error)
	SaveBoard(ctx context.Context, board datatug.Board) (*datatug.Board, error)
	GetBoard(ctx context.Context, id string) (board *datatug.Board, err error)
	DeleteBoard(ctx context.Context, id string) (err error)
}

package storage

import (
	"context"
	"github.com/datatug/datatug/packages/models"
)

// BoardsStore provides access to board records
type BoardsStore interface {
	ProjectStoreRef
	CreateBoard(ctx context.Context, board models.Board) (*models.Board, error)
	SaveBoard(ctx context.Context, board models.Board) (*models.Board, error)
	GetBoard(ctx context.Context, id string) (board *models.Board, err error)
	DeleteBoard(ctx context.Context, id string) (err error)
}

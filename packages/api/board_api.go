package api

import (
	"context"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
)

// GetBoard returns board by ID
func GetBoard(ctx context.Context, ref dto.ProjectItemRef) (*models.Board, error) {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).Boards().Board(ref.ID).LoadBoard(ctx)
}

// DeleteBoard deletes board
func DeleteBoard(ctx context.Context, ref dto.ProjectItemRef) error {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).Boards().Board(ref.ID).DeleteBoard(ctx)
}

// SaveBoard saves board
func SaveBoard(ctx context.Context, ref dto.ProjectItemRef, board models.Board) error {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).Boards().Board(ref.ID).SaveBoard(ctx, board)
}

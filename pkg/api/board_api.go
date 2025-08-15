package api

import (
	"context"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
	"log"
)

// CreateBoard creates board
func CreateBoard(ctx context.Context, ref dto.ProjectRef, board models.Board) (*models.Board, error) {
	log.Printf("api.CreateBoard(ref=%+v)", ref)
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	return store.GetProjectStore(ref.ProjectID).Boards().CreateBoard(ctx, board)
}

// GetBoard returns board by ID
func GetBoard(ctx context.Context, ref dto.ProjectItemRef) (*models.Board, error) {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	return store.GetProjectStore(ref.ProjectID).Boards().GetBoard(ctx, ref.ID)
}

// DeleteBoard deletes board
func DeleteBoard(ctx context.Context, ref dto.ProjectItemRef) error {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.GetProjectStore(ref.ProjectID).Boards().DeleteBoard(ctx, ref.ID)
}

// SaveBoard saves board
func SaveBoard(ctx context.Context, ref dto.ProjectRef, board models.Board) (*models.Board, error) {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	return store.GetProjectStore(ref.ProjectID).Boards().SaveBoard(ctx, board)
}

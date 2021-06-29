package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
)

// GetBoard returns board by ID
func GetBoard(projectItemRef dto.ProjectItemRef) (board models.Board, err error) {
	var dal storage.Store
	dal, err = storage.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.LoadBoard(projectItemRef.ProjectID, projectItemRef.ID)
}

// DeleteBoard deletes board
func DeleteBoard(projectItemRef dto.ProjectItemRef) (err error) {
	var dal storage.Store
	dal, err = storage.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.DeleteBoard(projectItemRef.ProjectID, projectItemRef.ID)
}

// SaveBoard saves board
func SaveBoard(projectItemRef dto.ProjectItemRef, board models.Board) (err error) {
	var dal storage.Store
	dal, err = storage.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.SaveBoard(projectItemRef.ProjectID, board)
}

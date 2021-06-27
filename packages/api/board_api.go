package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
)

// GetBoard returns board by ID
func GetBoard(projectItemRef ProjectItemRef) (board models.Board, err error) {
	var dal store.Interface
	dal, err = store.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.LoadBoard(projectItemRef.ProjectID, projectItemRef.ID)
}

// DeleteBoard deletes board
func DeleteBoard(projectItemRef ProjectItemRef) (err error) {
	var dal store.Interface
	dal, err = store.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.DeleteBoard(projectItemRef.ProjectID, projectItemRef.ID)
}

// SaveBoard saves board
func SaveBoard(projectItemRef ProjectItemRef, board models.Board) (err error) {
	var dal store.Interface
	dal, err = store.NewDatatugStore(projectItemRef.StoreID)
	if err != nil {
		return
	}
	return dal.SaveBoard(projectItemRef.ProjectID, board)
}

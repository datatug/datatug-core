package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
)

// GetBoard returns board by ID
func GetBoard(projectID, boardID string) (board models.Board, err error) {
	return store.Current.LoadBoard(projectID, boardID)
}

// DeleteBoard deletes board
func DeleteBoard(projectID, boardID string) (err error) {
	return store.Current.DeleteBoard(projectID, boardID)
}

// SaveBoard saves board
func SaveBoard(projectID string, board models.Board) (err error) {
	return store.Current.SaveBoard(projectID, board)
}

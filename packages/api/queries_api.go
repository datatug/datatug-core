package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
)

// GetQueries returns queries
func GetQueries(projectID, folder string) ([]models.Query, error) {
	return store.Current.LoadQueries(projectID, folder)
}

// SaveQuery saves query
func SaveQuery(projectID string, query models.Query) error {
	return store.Current.SaveQuery(projectID, query)
}

// DeleteQuery deletes query
func DeleteQuery(projectID, queryID string) error {
	return store.Current.DeleteQuery(projectID, queryID)
}

package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"net/http"
)

// GetEntity handles get entity endpoint
func GetEntity(w http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get(urlQueryParamProjectID)
	id := request.URL.Query().Get(urlQueryParamID)
	v, err := api.GetEntity(projectID, id)
	ReturnJSON(w, request, http.StatusOK, err, v)
}

// GetEntities returns list of project entities
func GetEntities(w http.ResponseWriter, request *http.Request) {
	projectID := request.URL.Query().Get(urlQueryParamProjectID)
	v, err := api.GetAllEntities(projectID)
	ReturnJSON(w, request, http.StatusOK, err, v)
}

// SaveEntity handles save entity endpoint
func SaveEntity(w http.ResponseWriter, request *http.Request) {
	var entity models.Entity
	saveFunc := func(projectID string) error {
		return api.SaveEntity(projectID, &entity)
	}
	saveItem(w, request, &entity, saveFunc)
}

// DeleteEntity handles delete entity endpoint
func DeleteEntity(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "entity", api.DeleteEntity)
}

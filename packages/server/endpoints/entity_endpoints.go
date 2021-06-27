package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"net/http"
)

// GetEntity handles get entity endpoint
func GetEntity(w http.ResponseWriter, request *http.Request) {
	ref := newProjectItemRef(request.URL.Query())
	v, err := api.GetEntity(ref)
	returnJSON(w, request, http.StatusOK, err, v)
}

// GetEntities returns list of project entities
func GetEntities(w http.ResponseWriter, request *http.Request) {
	ref := newProjectRef(request.URL.Query())
	v, err := api.GetAllEntities(ref)
	returnJSON(w, request, http.StatusOK, err, v)
}

// SaveEntity handles save entity endpoint
func SaveEntity(w http.ResponseWriter, request *http.Request) {
	var entity models.Entity
	saveFunc := func(ref api.ProjectItemRef) (interface{}, error) {
		entity.ID = ref.ID
		return entity, api.SaveEntity(ref.ProjectRef, &entity)
	}
	saveItem(w, request, &entity, saveFunc)
}

// DeleteEntity handles delete entity endpoint
func DeleteEntity(w http.ResponseWriter, request *http.Request) {
	deleteItem(w, request, "entity", api.DeleteEntity)
}

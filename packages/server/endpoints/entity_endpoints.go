package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"net/http"
)

// GetEntity handles get entity endpoint
func GetEntity(w http.ResponseWriter, r *http.Request) {
	ref := newProjectItemRef(r.URL.Query())
	v, err := api.GetEntity(r.Context(), ref)
	returnJSON(w, r, http.StatusOK, err, v)
}

// GetEntities returns list of project entities
func GetEntities(w http.ResponseWriter, r *http.Request) {
	ref := newProjectRef(r.URL.Query())
	v, err := api.GetAllEntities(r.Context(), ref)
	returnJSON(w, r, http.StatusOK, err, v)
}

// SaveEntity handles save entity endpoint
func SaveEntity(w http.ResponseWriter, request *http.Request) {
	var entity models.Entity
	saveFunc := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		entity.ID = ref.ID
		return entity, api.SaveEntity(ctx, ref.ProjectRef, &entity)
	}
	saveItem(w, request, &entity, saveFunc)
}

var DeleteEntity = deleteProjItem(api.DeleteEntity)

package api

import (
	"context"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
)

//// GetQueries returns queries
//func GetQueries(ctx context.Context, ref dto.ProjectRef, folder string) (*models.QueryFolder, error) {
//	store, err := storage.GetStore(ctx, ref.StoreID)
//	if err != nil {
//		return nil, err
//	}
//	//goland:noinspection GoNilness
//	project := store.GetProjectStore(ref.ProjectID)
//	return project.Queries().LoadQueries(ctx, folder)
//}

// CreateQuery creates a new query
func CreateQuery(ctx context.Context, request dto.CreateQuery) (*models.QueryDefWithFolderPath, error) {
	if err := request.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.GetStore(ctx, request.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(request.ProjectID)
	return project.Queries().CreateQuery(ctx, request.Query)
}

// UpdateQuery updates existing query
func UpdateQuery(ctx context.Context, request dto.UpdateQuery) (*models.QueryDefWithFolderPath, error) {
	if err := request.Validate(); err != nil {
		return nil, validation.NewBadRequestError(err)
	}
	store, err := storage.GetStore(ctx, request.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(request.ProjectID)
	return project.Queries().UpdateQuery(ctx, request.Query)
}

// DeleteQuery deletes query
func DeleteQuery(ctx context.Context, ref dto.ProjectItemRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(ref.ProjectID)
	return project.Queries().DeleteQuery(ctx, ref.ID)
}

// GetQuery returns query definition
func GetQuery(ctx context.Context, ref dto.ProjectItemRef) (query *models.QueryDefWithFolderPath, err error) {
	if err = ref.Validate(); err != nil {
		return query, err
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.GetProjectStore(ref.ProjectID)
	return project.Queries().GetQuery(ctx, ref.ID)
}

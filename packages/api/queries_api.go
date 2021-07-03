package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// GetQueries returns queries
func GetQueries(ref dto.ProjectRef, folder string) (*models.QueryFolder, error) {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Queries().LoadQueries(folder)
}

// CreateQueryFolder creates a new folder for queries
func CreateQueryFolder(request dto.CreateFolder) (*models.QueryFolder, error) {
	if err := request.ProjectRef.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.GetStore(request.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(request.ProjectID)
	return project.Queries().CreateQueryFolder(request.Path, request.Name)
}

// CreateQuery creates a new query
func CreateQuery(request dto.CreateQuery) error {
	if err := request.ProjectRef.Validate(); err != nil {
		return err
	}
	store, err := storage.GetStore(request.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(request.ProjectID)
	return project.Queries().CreateQuery(request.Query)
}

// UpdateQuery updates existing query
func UpdateQuery(request dto.UpdateQuery) error {
	if err := request.Validate(); err != nil {
		return validation.NewBadRequestError(err)
	}
	store, err := storage.GetStore(request.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(request.ProjectID)
	return project.Queries().Query(request.Query.ID).UpdateQuery(request.Query)
}

// DeleteQueryFolder deletes queries folder
func DeleteQueryFolder(ref dto.ProjectItemRef) error {
	if ref.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Queries().DeleteQueryFolder(ref.ID)
}

// DeleteQuery deletes query
func DeleteQuery(ref dto.ProjectItemRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Queries().Query(ref.ID).DeleteQuery()
}

// GetQuery returns query definition
func GetQuery(ref dto.ProjectItemRef) (query *models.QueryDef, err error) {
	if err = ref.Validate(); err != nil {
		return query, err
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Queries().Query(ref.ID).LoadQuery()
}

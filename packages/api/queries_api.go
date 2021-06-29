package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// GetQueries returns queries
func GetQueries(ref dto.ProjectRef, folder string) (*models.QueryFolder, error) {
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadQueries(ref.ProjectID, folder)
}

// CreateQueryFolder creates a new folder for queries
func CreateQueryFolder(request dto.CreateFolder) (folder *models.QueryFolder, err error) {
	if err := request.ProjectRef.Validate(); err != nil {
		return nil, err
	}
	dal, err := storage.NewDatatugStore(request.StoreID)
	if err != nil {
		return
	}
	return dal.CreateQueryFolder(request.ProjectID, request.Path, request.Name)
}

// CreateQuery creates a new query
func CreateQuery(request dto.CreateQuery) error {
	if err := request.ProjectRef.Validate(); err != nil {
		return err
	}
	dal, err := storage.NewDatatugStore(request.StoreID)
	if err != nil {
		return err
	}
	return dal.CreateQuery(request.ProjectID, request.Query)
}

// UpdateQuery updates existing query
func UpdateQuery(request dto.UpdateQuery) error {
	if err := request.Validate(); err != nil {
		return validation.NewBadRequestError(err)
	}
	dal, err := storage.NewDatatugStore(request.StoreID)
	if err != nil {
		return err
	}
	return dal.UpdateQuery(request.ProjectID, request.Query)
}

// DeleteQueryFolder deletes queries folder
func DeleteQueryFolder(ref dto.ProjectItemRef) error {
	if ref.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.DeleteQueryFolder(ref.ProjectID, ref.ID)
}

// DeleteQuery deletes query
func DeleteQuery(ref dto.ProjectItemRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.DeleteQuery(ref.ProjectID, ref.ID)
}

// GetQuery returns query definition
func GetQuery(ref dto.ProjectItemRef) (query *models.QueryDef, err error) {
	if err = ref.Validate(); err != nil {
		return query, err
	}
	dal, err := storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadQuery(ref.ProjectID, ref.ID)
}

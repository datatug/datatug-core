package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// GetQueries returns queries
func GetQueries(ref ProjectRef, folder string) (*models.QueryFolder, error) {
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadQueries(ref.ProjectID, folder)
}

// CreateQueryFolder creates a new folder for queries
func CreateQueryFolder(ref ProjectRef, path, id string) (folder models.QueryFolder, err error) {
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.CreateQueryFolder(ref.ProjectID, path, id)
}

// CreateQuery creates a new query
func CreateQuery(ref ProjectItemRef, query models.QueryDef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.CreateQuery(ref.ProjectID, query)
}

// UpdateQuery updates existing query
func UpdateQuery(ref ProjectItemRef, query models.QueryDef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	if err := query.Validate(); err != nil {
		return err
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.UpdateQuery(ref.ProjectID, query)
}

// DeleteQueryFolder deletes queries folder
func DeleteQueryFolder(ref ProjectItemRef) error {
	if ref.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.DeleteQueryFolder(ref.ProjectID, ref.ID)
}

// DeleteQuery deletes query
func DeleteQuery(ref ProjectItemRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return err
	}
	return dal.DeleteQuery(ref.ProjectID, ref.ID)
}

// GetQuery returns query definition
func GetQuery(ref ProjectItemRef) (query *models.QueryDef, err error) {
	if err = ref.Validate(); err != nil {
		return query, err
	}
	dal, err := store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return nil, err
	}
	return dal.LoadQuery(ref.ProjectID, ref.ID)
}

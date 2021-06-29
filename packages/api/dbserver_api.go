package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(storeID, projectID string, projDbServer models.ProjDbServer) (err error) {
	store, err := storage.NewDatatugStore(storeID)
	if err != nil {
		return err
	}
	return store.SaveDbServer(projectID, projDbServer, models.DatatugProject{})
}

// UpdateDbServer adds db server to project
//goland:noinspection GoUnusedExportedFunction
func UpdateDbServer(ref dto.ProjectRef, projDbServer models.ProjDbServer) (err error) {
	var dal storage.Store
	dal, err = storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.SaveDbServer(ref.ProjectID, projDbServer, models.DatatugProject{})
}

// DeleteDbServer adds db server to project
func DeleteDbServer(ref dto.ProjectRef, dbServer models.ServerReference) (err error) {
	var dal storage.Store
	dal, err = storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.DeleteDbServer(ref.ProjectID, dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(ref dto.ProjectRef, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error) {
	if err = dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return
	}
	var dal storage.Store
	dal, err = storage.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	summary, err = dal.LoadDbServerSummary(ref.ProjectID, dbServer)
	return
}

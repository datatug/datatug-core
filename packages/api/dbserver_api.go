package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(storeId, projectID string, projDbServer models.ProjDbServer) (err error) {
	store, err := store.NewDatatugStore(storeId)
	if err != nil {
		return err
	}
	return store.SaveDbServer(projectID, projDbServer, models.DatatugProject{})
}

// AddDbServer adds db server to project
//goland:noinspection GoUnusedExportedFunction
func UpdateDbServer(ref ProjectRef, projDbServer models.ProjDbServer) (err error) {
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.SaveDbServer(ref.ProjectID, projDbServer, models.DatatugProject{})
}

// DeleteDbServer adds db server to project
func DeleteDbServer(ref ProjectRef, dbServer models.ServerReference) (err error) {
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.DeleteDbServer(ref.ProjectID, dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(ref ProjectRef, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error) {
	if err = dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return
	}
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	summary, err = dal.LoadDbServerSummary(ref.ProjectID, dbServer)
	return
}

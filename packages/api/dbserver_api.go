package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(projectID string, projDbServer models.ProjDbServer) (err error) {
	return store.Current.SaveDbServer(projectID, projDbServer, models.DatatugProject{})
}

// AddDbServer adds db server to project
//goland:noinspection GoUnusedExportedFunction
func UpdateDbServer(projectID string, projDbServer models.ProjDbServer) (err error) {
	return store.Current.SaveDbServer(projectID, projDbServer, models.DatatugProject{})
}

// DeleteDbServer adds db server to project
func DeleteDbServer(projectID string, dbServer models.ServerReference) (err error) {
	return store.Current.DeleteDbServer(projectID, dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(projID string, dbServer models.ServerReference) (summary *models.ProjDbServerSummary, err error) {
	if err = dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return
	}
	summary, err = store.Current.LoadDbServerSummary(projID, dbServer)
	return
}

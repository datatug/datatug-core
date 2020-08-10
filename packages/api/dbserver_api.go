package api

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"github.com/datatug/datatug/packages/store"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(projectID string, dbServer models.DbServer) (err error) {
	projDbServer := models.ProjDbServer{
		DbServer: dbServer,
	}
	return store.Current.SaveDbServer(projectID, &projDbServer)
}

// DeleteDbServer adds db server to project
func DeleteDbServer(projectID string, dbServer models.DbServer) (err error) {
	return store.Current.DeleteDbServer(projectID, dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(projID string, dbServer models.DbServer) (summary *dto.ProjDbServerSummary, err error) {
	if err = dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return
	}
	summary, err = store.Current.GetDbServerSummary(projID, dbServer)
	return
}

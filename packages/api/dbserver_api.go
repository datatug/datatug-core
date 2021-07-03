package api

import (
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(ref dto.ProjectRef, projDbServer models.ProjDbServer) error {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	dbServerStore := store.Project(ref.ProjectID).DbServers().DbServer(projDbServer.Server)
	return dbServerStore.SaveDbServer(projDbServer, models.DatatugProject{})
}

// UpdateDbServer adds db server to project
//goland:noinspection GoUnusedExportedFunction
func UpdateDbServer(ref dto.ProjectRef, projDbServer models.ProjDbServer) error {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).DbServers().DbServer(projDbServer.Server).SaveDbServer(projDbServer, models.DatatugProject{})
}

// DeleteDbServer adds db server to project
func DeleteDbServer(ref dto.ProjectRef, dbServer models.ServerReference) (err error) {
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).DbServers().DbServer(dbServer).DeleteDbServer(dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(ref dto.ProjectRef, dbServer models.ServerReference) (*models.ProjDbServerSummary, error) {
	if err := dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return nil, err
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	return store.Project(ref.ProjectID).DbServers().DbServer(dbServer).LoadDbServerSummary(dbServer)
}

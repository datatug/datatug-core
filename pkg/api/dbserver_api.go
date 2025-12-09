package api

import (
	"context"

	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/strongo/validation"
)

// AddDbServer adds db server to project
func AddDbServer(ctx context.Context, ref dto.ProjectRef, projDbServer models.ProjDbServer) error {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	dbServerStore := store.GetProjectStore(ref.ProjectID).DbServers().DbServer(projDbServer.Server)
	return dbServerStore.SaveDbServer(ctx, projDbServer, models.DatatugProject{})
}

// UpdateDbServer adds db server to project
//
//goland:noinspection GoUnusedExportedFunction
func UpdateDbServer(ctx context.Context, ref dto.ProjectRef, projDbServer models.ProjDbServer) error {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	dbServerStore := store.GetProjectStore(ref.ProjectID).DbServers().DbServer(projDbServer.Server)
	return dbServerStore.SaveDbServer(ctx, projDbServer, models.DatatugProject{})
}

// DeleteDbServer adds db server to project
func DeleteDbServer(ctx context.Context, ref dto.ProjectRef, dbServer models.ServerReference) (err error) {
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	return store.GetProjectStore(ref.ProjectID).DbServers().DbServer(dbServer).DeleteDbServer(ctx, dbServer)
}

// GetDbServerSummary returns summary on DB server
func GetDbServerSummary(ctx context.Context, ref dto.ProjectRef, dbServer models.ServerReference) (*models.ProjDbServerSummary, error) {
	if err := dbServer.Validate(); err != nil {
		err = validation.NewBadRequestError(err)
		return nil, err
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	return store.GetProjectStore(ref.ProjectID).DbServers().DbServer(dbServer).LoadDbServerSummary(ctx, dbServer)
}

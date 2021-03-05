package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/server/dto"
	"path"
)

func (loader fileSystemLoader) LoadDbCatalogSummary(projectID string, dbServer models.ServerReference, catalogID string) (*dto.DbCatalogSummary, error) {
	projPath := loader.pathByID[projectID]
	catalogsDirPath := path.Join(projPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Driver, dbServer.Host, DbCatalogsFolder)
	return loadDbCatalogSummary(catalogsDirPath, catalogID)
}

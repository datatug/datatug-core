package filestore

import (
	"github.com/datatug/datatug/packages/models"
	"path"
)

func (loader fileSystemLoader) LoadDbCatalogSummary(projectID string, dbServer models.ServerReference, catalogID string) (*models.DbCatalogSummary, error) {
	projPath := loader.pathByID[projectID]
	catalogsDirPath := path.Join(projPath, DatatugFolder, ServersFolder, DbFolder, dbServer.Driver, dbServer.Host, DbCatalogsFolder)
	return loadDbCatalogSummary(catalogsDirPath, catalogID)
}

package models2md

import (
	"fmt"
	"io"

	"github.com/datatug/datatug-core/pkg/models"
)

// NewEncoder creates new encoder
func NewEncoder() models.ReadmeEncoder {
	return encoder{}
}

type encoder struct {
}

func (encoder) EnvironmentsToReadme(w io.Writer, environments *models.Environments) error {
	return writeReadme(w, "environments.md", map[string]interface{}{
		"environments": environments,
	})
}

func (encoder) DbServerToReadme(w io.Writer, _ *models.ProjectRepository, dbServer models.ProjDbServer) error {
	return writeReadme(w, "dbserver.md", map[string]interface{}{
		"dbServer": dbServer,
	})
}

func (encoder) DbCatalogToReadme(w io.Writer, _ *models.ProjectRepository, dbServer models.ProjDbServer, catalog models.DbCatalog) error {
	return writeReadme(w, "dbserver.md", map[string]interface{}{
		"dbServer": dbServer,
	})
}

func (encoder) TableToReadme(w io.Writer, repository *models.ProjectRepository, catalog string, table *models.Table, dbServer models.ProjDbServer) error {
	data, err := getTableData(repository, catalog, table, dbServer)
	if err != nil {
		return fmt.Errorf("failed to get data for table template: %w", err)
	}
	return writeReadme(w, "table.md", data)
}

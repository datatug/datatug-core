package datatug2md

import (
	"fmt"
	"io"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// NewEncoder creates new encoder
func NewEncoder() datatug.ReadmeEncoder {
	return encoder{}
}

type encoder struct {
}

func (encoder) EnvironmentsToReadme(w io.Writer, environments *datatug.Environments) error {
	return writeReadme(w, "environments.md", map[string]interface{}{
		"environments": environments,
	})
}

func (encoder) DbServerToReadme(w io.Writer, _ *datatug.ProjectRepository, dbServer datatug.ProjDbServer) error {
	return writeReadme(w, "dbserver.md", map[string]interface{}{
		"dbServer": dbServer,
	})
}

func (encoder) DbCatalogToReadme(w io.Writer, _ *datatug.ProjectRepository, dbServer datatug.ProjDbServer, catalog datatug.EnvDbCatalog) error {
	return writeReadme(w, "dbserver.md", map[string]interface{}{
		"dbServer": dbServer,
		"catalog":  catalog,
	})
}

func (encoder) TableToReadme(w io.Writer, repository *datatug.ProjectRepository, catalog string, table *datatug.CollectionInfo, dbServer datatug.ProjDbServer) error {
	data, err := getTableData(repository, catalog, table, dbServer)
	if err != nil {
		return fmt.Errorf("failed to get data for table template: %w", err)
	}
	return writeReadme(w, "table.md", data)
}

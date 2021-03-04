package models

import "io"

// ReadmeEncoder defines an interface for encoder implementation that writes to MD files
type ReadmeEncoder interface {
	ProjectSummaryToReadme(w io.Writer, project DataTugProject) error
	DbServerToReadme(w io.Writer, repository *ProjectRepository, dbServer ProjDbServer) error
	TableToReadme(w io.Writer, repository *ProjectRepository, catalog string, table *Table, dbServer ProjDbServer) error
	DbCatalogToReadme(w io.Writer, repository *ProjectRepository, dbServer ProjDbServer, catalog DbCatalog) error
}

package datatug

import "io"

// ReadmeEncoder defines an interface for encoder implementation that writes to MD files
type ReadmeEncoder interface {
	ProjectSummaryToReadme(w io.Writer, project Project) error
	DbServerToReadme(w io.Writer, repository *ProjectRepository, dbServer ProjDbServer) error
	TableToReadme(w io.Writer, repository *ProjectRepository, catalog string, table *CollectionInfo, dbServer ProjDbServer) error
	DbCatalogToReadme(w io.Writer, repository *ProjectRepository, dbServer ProjDbServer, catalog EnvDbCatalog) error
}

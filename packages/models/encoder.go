package models

import "io"

// ReadmeEncoder defines an interface for encoder implementation that writes to MD files
type ReadmeEncoder interface {
	EncodeProjectSummary(w io.Writer, project DataTugProject) error
	EncodeTable(w io.Writer, catalog string, table *Table, dbServer ProjDbServer) error
}

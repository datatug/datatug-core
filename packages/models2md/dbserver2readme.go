package models2md

import (
	"embed"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"io"
	"text/template"
)

//go:embed dbServer.md
var dbServerTemplate embed.FS

func (encoder) DbServerToReadme(w io.Writer, _ *models.ProjectRepository, dbServer models.ProjDbServer) error {
	t, err := template.New("dbServer_README.md").ParseFS(dbServerTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse template for DB server: %w", err)
	}
	if err = t.Execute(w, map[string]interface{}{
		"dbServer": dbServer,
	}); err != nil {
		return fmt.Errorf("failed to write DB server info to README.md")
	}
	return nil
}

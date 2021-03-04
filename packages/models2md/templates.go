package models2md

import (
	"embed"
	"fmt"
	"io"
	"text/template" // TODO: use "html/template"
)

//go:embed templates/*.md
var templatesFS embed.FS

func writeReadme(w io.Writer, name string, data map[string]interface{}) error {
	t, err := template.New(name).ParseFS(templatesFS, "templates/*.md")
	if err != nil {
		return fmt.Errorf("failed to parse templates for %v server: %w", name, err)
	}
	if err = t.Execute(w, data); err != nil {
		return fmt.Errorf("failed to write into %v: %w", name, err)
	}
	return nil
}

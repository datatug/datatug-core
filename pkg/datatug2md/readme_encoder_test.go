package datatug2md

import (
	"bytes"
	"io"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestEncoder_ProjectSummaryToReadme(t *testing.T) {
	encoder := NewEncoder()
	project := datatug.Project{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: "test-project",
			},
		},
		DbModels: datatug.DbModels{
			{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "db1"}}},
		},
		Environments: datatug.Environments{
			{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "env1"}}},
		},
		Boards: datatug.Boards{
			{ProjBoardBrief: datatug.ProjBoardBrief{ProjItemBrief: datatug.ProjItemBrief{ID: "board1", Title: "Board 1"}}},
		},
	}

	w := new(bytes.Buffer)
	err := encoder.ProjectSummaryToReadme(w, project)
	if err != nil {
		t.Fatalf("ProjectSummaryToReadme failed: %v", err)
	}

	output := w.String()
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestEncoder_EnvironmentsToReadme(t *testing.T) {
	encoder := NewEncoder().(interface {
		EnvironmentsToReadme(w io.Writer, environments *datatug.Environments) error
	})
	environments := &datatug.Environments{
		{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "env1"}}},
	}

	w := new(bytes.Buffer)
	err := encoder.EnvironmentsToReadme(w, environments)
	if err != nil {
		t.Fatalf("EnvironmentsToReadme failed: %v", err)
	}

	if w.String() == "" {
		t.Error("expected non-empty output")
	}
}

func TestEncoder_DbServerToReadme(t *testing.T) {
	encoder := NewEncoder()
	dbServer := datatug.ProjDbServer{
		Server: datatug.ServerReference{
			Driver: "sqlserver",
			Host:   "localhost",
		},
	}

	w := new(bytes.Buffer)
	err := encoder.DbServerToReadme(w, nil, dbServer)
	if err != nil {
		t.Fatalf("DbServerToReadme failed: %v", err)
	}

	if w.String() == "" {
		t.Error("expected non-empty output")
	}
}

func TestEncoder_DbCatalogToReadme(t *testing.T) {
	encoder := NewEncoder()
	dbServer := datatug.ProjDbServer{}
	catalog := datatug.EnvDbCatalog{}

	w := new(bytes.Buffer)
	err := encoder.DbCatalogToReadme(w, nil, dbServer, catalog)
	if err != nil {
		t.Fatalf("DbCatalogToReadme failed: %v", err)
	}

	if w.String() == "" {
		t.Error("expected non-empty output")
	}
}

func TestEncoder_TableToReadme(t *testing.T) {
	encoder := NewEncoder()
	dbServer := datatug.ProjDbServer{}
	table := &datatug.CollectionInfo{
		Columns: datatug.TableColumns{
			{
				DbColumnProps: datatug.DbColumnProps{
					Name:   "col1",
					DbType: "nvarchar",
				},
			},
		},
	}

	w := new(bytes.Buffer)
	err := encoder.TableToReadme(w, nil, "test-catalog", table, dbServer)
	if err != nil {
		t.Fatalf("TableToReadme failed: %v", err)
	}

	if w.String() == "" {
		t.Error("expected non-empty output")
	}
}

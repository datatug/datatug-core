package datatug2md

import (
	"fmt"
	"io"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// ProjectSummaryToReadme encodes project summary to markdown file format
func (encoder) ProjectSummaryToReadme(w io.Writer, project datatug.Project) error {
	dbModels := make([]string, len(project.DbModels))
	for i, dbModel := range project.DbModels {
		dbModels[i] = fmt.Sprintf("- [%v](dbmodels/%v)", dbModel.ID, dbModel.ID)
	}

	environments := make([]string, len(project.Environments))
	for i, environment := range project.Environments {
		environments[i] = fmt.Sprintf("- [%v](dbmodels/%v)", environment.ID, environment.ID)
	}

	boards := make([]string, len(project.Boards))
	for i, board := range project.Boards {
		boards[i] = fmt.Sprintf("- [%v](boards/%v)", board.Title, board.ID)
	}

	return writeReadme(w, "project.md", map[string]interface{}{
		"project":      project,
		"dbModels":     strings.Join(dbModels, "\n"),
		"environments": strings.Join(environments, "\n"),
		"boards":       strings.Join(boards, "\n"),
	})
}

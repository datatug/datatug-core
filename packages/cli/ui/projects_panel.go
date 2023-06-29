package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/rivo/tview"
	"strconv"
)

type ProjectsList struct {
	tview.Primitive
}

func NewProjectsList(app *tview.Application) (*ProjectsList, error) {
	projects := tview.NewList()
	settings, err := config.GetSettings()
	if err != nil {
		return nil, err
	}
	projectNumber := 0
	for id, project := range settings.Projects {
		projectNumber++
		projects.AddItem(id, project.Path, rune(strconv.Itoa(projectNumber)[0]), func() {
			homeScreen := NewProjectScreen(app, project)
			app.SetRoot(homeScreen, true)
		})
	}

	projects.SetTitle("Projects") // TODO(ask-stackoverflow): how to set title?

	menu := &ProjectsList{
		Primitive: projects,
	}
	return menu, nil
}

package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/rivo/tview"
	"sort"
	"strconv"
)

type ProjectsList struct {
	tview.Primitive
}

func NewProjectsList(app *tview.Application) (*ProjectsList, error) {
	projectsList := tview.NewList()
	settings, err := config.GetSettings()
	if err != nil {
		return nil, err
	}

	openProject := func(projectConfig config.ProjectConfig) {
		homeScreen := NewProjectScreen(app, projectConfig)
		app.SetRoot(homeScreen, true)
	}

	projects := make([]config.ProjectConfig, 0, len(settings.Projects))

	for _, p := range settings.Projects {
		projects = append(projects, p)
	}

	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ID < projects[j].ID
	})

	for i, p := range projects {
		project := p
		projectsList.AddItem(project.ID, project.Path, rune(strconv.Itoa(i + 1)[0]), func() {
			openProject(project)
		})
	}

	projectsList.SetTitle("Projects") // TODO(ask-stackoverflow): how to set title?

	menu := &ProjectsList{
		Primitive: projectsList,
	}
	return menu, nil
}

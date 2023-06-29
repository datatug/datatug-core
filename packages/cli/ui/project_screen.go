package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/rivo/tview"
)

type projectScreen struct {
	*tview.Grid
}

func NewProjectScreen(app *tview.Application, project config.ProjectConfig) tview.Primitive {
	screen := new(homeScreen)

	menu := tview.NewList().
		AddItem("Queries", "", 'q', nil).
		AddItem("Environments", "", 'e', nil)

	menu.SetBorderPadding(0, 0, 1, 1)

	//sideBar := NewProjectsMenu()

	header := NewHeaderPanel(project.ID)

	footer := NewFooterPanel()

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(20, 0).
		SetBorders(true).
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false)

	projects, err := NewProjectsList(app)
	if err != nil {
		panic(err)
	}

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(projects, 1, 0, 1, 3, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(projects, 1, 1, 1, 1, 0, 100, false)

	screen.Primitive = grid
	return screen
}

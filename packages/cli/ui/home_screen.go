package ui

import (
	"github.com/rivo/tview"
)

func NewHomeScreen(app *tview.Application) tview.Primitive {
	screen := new(homeScreen)

	menu := tview.NewList().
		AddItem("Projects", "", 'p', nil).
		AddItem("Settings", "", 's', nil)

	menu.SetBorderPadding(0, 0, 1, 1)

	sideBar := NewProjectsMenu()

	header := NewHeaderPanel("")

	footer := NewFooterPanel()

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(20, 0, 20).
		SetBorders(true).
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false)

	projects, err := NewProjectsList(app)
	if err != nil {
		panic(err)
	}

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(projects, 1, 0, 1, 3, 0, 0, false).
		AddItem(sideBar, 0, 0, 0, 0, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(projects, 1, 1, 1, 1, 0, 100, false).
		AddItem(sideBar, 1, 2, 1, 1, 0, 100, false)

	screen.Primitive = grid
	return screen
}

type homeScreen struct {
	tview.Primitive
}

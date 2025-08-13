package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
)

func newProjectsScreen(tui *tapp2.TUI) tapp2.Screen {

	header := newHeaderPanel(tui, "")

	grid := layoutGrid(header)

	projectsPanel, err := newProjectsPanel(tui)
	if err != nil {
		panic(err)
	}

	menu := newHomeMenu(tui, projectsRootScreen)
	sidebar := newProjectsMenu(tui)

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.
		AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(projectsPanel, 1, 0, 1, 3, 0, 0, false).
		AddItem(sidebar, 0, 0, 0, 0, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.
		AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(projectsPanel, 1, 1, 1, 1, 0, 100, false).
		AddItem(sidebar, 1, 2, 1, 1, 0, 100, false)

	grid.SetFocusFunc(func() {
		menu.TakeFocus()
	})

	_ = tapp2.NewRow(tui.App,
		menu,
		projectsPanel,
		sidebar,
	)

	screen := &projectsScreen{
		ScreenBase: tapp2.NewScreenBase(tui, grid, tapp2.FullScreen()),
	}

	tui.SetRootScreen(screen)

	screen.TakeFocus()

	return screen
}

var _ tapp2.Screen = (*projectsScreen)(nil)

type projectsScreen struct {
	tapp2.ScreenBase
	//row *tapp.Row
}

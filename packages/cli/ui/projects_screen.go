package ui

import (
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

func newProjectsScreen(tui *tapp.TUI) tapp.Screen {

	header := newHeaderPanel(tui, "")
	menu := newHomeMenu(tui, projectsRootScreen)
	sideBar := newProjectsMenu(tui)
	footer := NewFooterPanel()

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(20, 0, 20).
		SetBorders(false).
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false)

	projectsPanel, err := newProjectsPanel(tui)
	if err != nil {
		panic(err)
	}

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.
		AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(projectsPanel, 1, 0, 1, 3, 0, 0, false).
		AddItem(sideBar, 0, 0, 0, 0, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.
		AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(projectsPanel, 1, 1, 1, 1, 0, 100, false).
		AddItem(sideBar, 1, 2, 1, 1, 0, 100, false)

	grid.SetFocusFunc(func() {
		menu.TakeFocus()
	})

	_ = tapp.NewRow(tui.App,
		menu,
		projectsPanel,
		sideBar,
	)

	screen := &projectsScreen{
		ScreenBase: tapp.NewScreenBase(tui, grid, tapp.FullScreen()),
	}

	tui.SetRootScreen(screen)

	screen.TakeFocus()

	return screen
}

var _ tapp.Screen = (*projectsScreen)(nil)

type projectsScreen struct {
	tapp.ScreenBase
	row *tapp.Row
}

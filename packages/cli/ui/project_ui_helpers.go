package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

func newProjectRootScreenBase(
	tui *tapp.TUI,
	project config.ProjectConfig,
	screen ProjectScreenID,
	main tapp.Panel,
	sidebar tapp.Panel,
) tapp.ScreenBase {
	grid := projectScreenGreed(tui, project, screen, main, sidebar)

	screenBase := tapp.NewScreenBase(tui, grid, tapp.FullScreen())

	screenBase.TakeFocus()

	return screenBase
}

func projectScreenGreed(
	tui *tapp.TUI,
	project config.ProjectConfig,
	screenID ProjectScreenID,
	main tapp.Panel,
	sidebar tapp.Panel,
) *tview.Grid {
	menu := newProjectMenu(tui, project, screenID)

	header := newHeaderPanel(tui, project.ID)

	footer := NewFooterPanel()

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(20, 0, 20).
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false).
		AddItem(sidebar, 1, 0, 1, 1, 0, 0, false)

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.
		AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(main, 1, 0, 1, 3, 0, 0, false).
		AddItem(sidebar, 0, 0, 0, 0, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(main, 1, 1, 1, 1, 0, 100, false).
		AddItem(sidebar, 1, 2, 1, 1, 0, 100, false)

	grid.SetFocusFunc(func() {
		menu.TakeFocus()
	})

	_ = tapp.NewRow(tui.App, menu, main, sidebar)
	return grid
}

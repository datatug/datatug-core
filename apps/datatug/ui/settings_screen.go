package ui

import (
	"github.com/datatug/datatug/apps/datatug/config"
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/rivo/tview"
	"gopkg.in/yaml.v3"
)

type settingsScreen struct {
	tapp2.ScreenBase
}

func NewSettingsScreen(tui *tapp2.TUI) tapp2.Screen {
	header := newHeaderPanel(tui, "")
	menu := newHomeMenu(tui, settingsRootScreen)
	sideBar := newProjectsMenu(tui)
	footer := NewFooterPanel()

	grid := tview.NewGrid().
		SetRows(1, 0, 1).
		SetColumns(20, 0, 20).
		SetBorders(false).
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false)

	setting, _ := config.GetSettings()

	content, _ := yaml.Marshal(setting)

	const fileName = " Config File: ~/.datatug.yaml"
	settingsPanel := tview.NewTextView().SetText(string(content))
	defaultBorder(settingsPanel.Box)
	settingsPanel.SetTitle(fileName)
	settingsPanel.SetTitleAlign(tview.AlignLeft)

	// Layout for screens narrower than 100 cells (menu and sidebar are hidden).
	grid.AddItem(menu, 0, 0, 0, 0, 0, 0, false).
		AddItem(settingsPanel, 1, 0, 1, 3, 0, 0, false).
		AddItem(sideBar, 0, 0, 0, 0, 0, 0, false)

	// Layout for screens wider than 100 cells.
	grid.AddItem(menu, 1, 0, 1, 1, 0, 100, false).
		AddItem(settingsPanel, 1, 1, 1, 1, 0, 100, false).
		AddItem(sideBar, 1, 2, 1, 1, 0, 100, false)

	return &settingsScreen{
		ScreenBase: tapp2.NewScreenBase(tui, grid, tapp2.FullScreen()),
	}
}

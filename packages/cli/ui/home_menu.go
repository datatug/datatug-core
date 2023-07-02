package ui

import (
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

type rootScreen int

const (
	projectsRootScreen rootScreen = iota
	settingsRootScreen
)

func newHomeMenu(tui *tapp.TUI, active rootScreen) (menu *homeMenu) {
	newRootScreen := func(newScreen func(tui2 *tapp.TUI) tapp.Screen) func() {
		return func() {
			screen := newScreen(tui)
			tui.SetRootScreen(screen)
		}
	}

	list := tview.NewList().SetWrapAround(false).
		AddItem("Projects", "", 'P', newRootScreen(newProjectsScreen)).
		AddItem("Settings", "", 'S', newRootScreen(NewSettingsScreen))
	list.SetCurrentItem(int(active))

	defaultBorder(list.Box)

	menu = &homeMenu{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
		list:      list,
	}

	return menu
}

var _ tapp.Cell = (*homeMenu)(nil)

type homeMenu struct {
	tapp.PanelBase
	list *tview.List
}

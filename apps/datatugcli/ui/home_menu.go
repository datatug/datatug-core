package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatugcli/tapp"
	"github.com/rivo/tview"
)

type rootScreen int

const (
	projectsRootScreen rootScreen = iota
	settingsRootScreen
)

func newHomeMenu(tui *tapp2.TUI, active rootScreen) (menu *homeMenu) {
	newRootScreen := func(newScreen func(tui2 *tapp2.TUI) tapp2.Screen) func() {
		return func() {
			screen := newScreen(tui)
			tui.SetRootScreen(screen)
		}
	}
	list := menuList().
		AddItem("Projects", "", 'P', newRootScreen(newProjectsScreen)).
		AddItem("Settings", "", 'S', newRootScreen(NewSettingsScreen))
	list.SetCurrentItem(int(active))

	defaultBorder(list.Box)

	menu = &homeMenu{
		PanelBase: tapp2.NewPanelBase(tui, list, list.Box),
		list:      list,
	}

	return menu
}

var _ tapp2.Cell = (*homeMenu)(nil)

type homeMenu struct {
	tapp2.PanelBase
	list *tview.List
}

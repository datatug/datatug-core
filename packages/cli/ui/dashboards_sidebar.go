package ui

import "github.com/datatug/datatug/packages/cli/tapp"

type dashboardsSubMenu struct {
	tapp.PanelBase
}

func newDashboardsSidebar(tui *tapp.TUI) *dashboardsSubMenu {
	list := menuList()

	list.
		AddItem("Add", "", 'A', func() {
			panic("implement me")
		})

	menu := &dashboardsSubMenu{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
	}

	return menu
}

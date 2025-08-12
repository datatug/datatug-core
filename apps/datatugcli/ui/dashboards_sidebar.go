package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatugcli/tapp"
)

type dashboardsSubMenu struct {
	tapp2.PanelBase
}

func newDashboardsSidebar(tui *tapp2.TUI) *dashboardsSubMenu {
	list := menuList()

	list.
		AddItem("Add", "", 'A', func() {
			panic("implement me")
		})

	menu := &dashboardsSubMenu{
		PanelBase: tapp2.NewPanelBase(tui, list, list.Box),
	}

	return menu
}

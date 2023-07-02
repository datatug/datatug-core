package ui

import (
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

func newProjectMenu(tui *tapp.TUI) *projectMenu {
	list := tview.NewList().
		AddItem("Databases", "", 'D', nil).
		AddItem("Dashboards", "", 'B', nil).
		AddItem("Environments", "", 'E', nil).
		AddItem("Queries", "", 'Q', nil).
		AddItem("Web UI", "", 'W', nil)

	list.SetCurrentItem(2)

	defaultListStyle(list)

	return &projectMenu{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
	}
}

type projectMenu struct {
	tapp.PanelBase
}

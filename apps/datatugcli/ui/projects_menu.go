package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatugcli/tapp"
)

func newProjectsMenu(tui *tapp2.TUI) *projectsMenu {
	list := menuList()
	list.
		AddItem("Add", "", 'A', nil).
		AddItem("Delete", "", 'D', nil)
	defaultListStyle(list)
	menu := &projectsMenu{
		PanelBase: tapp2.NewPanelBase(tui, list, list.Box),
	}
	return menu
}

type projectsMenu struct {
	tapp2.PanelBase
}

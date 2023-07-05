package ui

import (
	"github.com/datatug/datatug/packages/cli/tapp"
)

func newProjectsMenu(tui *tapp.TUI) *projectsMenu {
	list := menuList()
	list.
		AddItem("Add", "", 'A', nil).
		AddItem("Delete", "", 'D', nil)
	defaultListStyle(list)
	menu := &projectsMenu{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
	}
	return menu
}

type projectsMenu struct {
	tapp.PanelBase
}

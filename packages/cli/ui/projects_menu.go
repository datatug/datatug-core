package ui

import (
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

func newProjectsMenu(tui *tapp.TUI) *projectsMenu {
	list := tview.NewList().SetWrapAround(false).
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

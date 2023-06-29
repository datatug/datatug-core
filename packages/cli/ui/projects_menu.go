package ui

import "github.com/rivo/tview"

func NewProjectsMenu() (menu tview.Primitive) {
	list := tview.NewList().
		AddItem("Add", "", 'a', nil).
		AddItem("Delete", "", 'd', nil)
	list.SetBorderPadding(0, 0, 1, 1)
	menu = &projectsMenu{
		Primitive: list,
	}
	return menu
}

type projectsMenu struct {
	tview.Primitive
}

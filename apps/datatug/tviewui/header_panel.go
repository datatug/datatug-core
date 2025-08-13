package ui

import (
	"github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/rivo/tview"
)

func newHeaderPanel(tui *tapp.TUI, project string) (header *headerPanel) {
	flex := tview.NewFlex()

	home := tview.NewButton("DataTug")
	home.SetSelectedFunc(func() {
		for tui.StackDepth() > 1 {
			tui.PopScreen()
		}
	})
	//home.SetBorderPadding(0, 0, 1, 1)

	flex.AddItem(home, 9, 1, false)

	if project != "" {
		projectCrumb := tview.NewTextView().SetText(" > " + project)
		flex.AddItem(projectCrumb, 0, 2, false)
	}
	header = &headerPanel{
		Primitive: flex,
	}
	return header
}

type headerPanel struct {
	tview.Primitive
}

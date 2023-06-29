package ui

import "github.com/rivo/tview"

func newHeaderPanel(app *tview.Application, project string) (header *headerPanel) {
	flex := tview.NewFlex()

	home := tview.NewButton("DataTug")
	home.SetSelectedFunc(func() {
		app.SetRoot(NewHomeScreen(app), true)
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

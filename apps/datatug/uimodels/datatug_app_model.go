package uimodels

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps"
)

var _ tea.Model = (*datatugAppModel)(nil)

type datatugAppModel struct {
	apps.BaseAppModel
}

func DatatugAppModel() tea.Model {
	app := &datatugAppModel{}
	app.NavStack.SetRoot(newDatatugMainMenu())
	return app
}

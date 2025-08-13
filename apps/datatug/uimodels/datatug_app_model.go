package uimodels

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps"
	"github.com/datatug/datatug/packages/bubbles/panel"
)

var _ tea.Model = (*datatugAppModel)(nil)

type datatugAppModel struct {
	apps.BaseAppModel
}

func DatatugAppModel() tea.Model {
	app := &datatugAppModel{}
	app.Panels = []panel.Panel{
		panel.New(newDatatugMainMenu(), "DataTug"),
		panel.New(newViewersModel(nil), "Viewers 2"),
	}
	return app
}

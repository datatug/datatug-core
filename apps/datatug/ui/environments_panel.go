package ui

import (
	"github.com/datatug/datatug/packages/appconfig"
	"github.com/rivo/tview"
)

type environmentsPanel struct {
	projectBasePanel
}

func newEnvironmentsPanel(project appconfig.ProjectConfig) *environmentsPanel {

	content := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("List of environments here")

	defaultBorder(content.Box)

	return &environmentsPanel{
		projectBasePanel: newProjectBasePanel(project, content, content.Box),
	}
}

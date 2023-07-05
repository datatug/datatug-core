package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

type environmentsPanel struct {
	projectBasePanel
}

func newEnvironmentsPanel(tui *tapp.TUI, project config.ProjectConfig) *environmentsPanel {

	content := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("List of environments here")

	defaultBorder(content.Box)

	return &environmentsPanel{
		projectBasePanel: newProjectBasePanel(tui, project, content, content.Box),
	}
}

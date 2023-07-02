package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

type projectPanel struct {
	tapp.PanelBase
}

func newProjectPanel(tui *tapp.TUI, project config.ProjectConfig) *projectPanel {

	content := tview.NewTextView().SetTextAlign(tview.AlignCenter).SetText("Main content")

	defaultBorder(content.Box)

	return &projectPanel{
		PanelBase: tapp.NewPanelBase(tui, content, content.Box),
	}
}

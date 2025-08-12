package ui

import (
	"github.com/datatug/datatug/apps/datatugcli/config"
	"github.com/datatug/datatug/apps/datatugcli/tapp"
	"github.com/rivo/tview"
)

type projectBasePanel struct {
	project config.ProjectConfig
	tapp.PanelBase
}

func newProjectBasePanel(project config.ProjectConfig, primitive tview.Primitive, box *tview.Box) projectBasePanel {
	return projectBasePanel{
		project:   project,
		PanelBase: tapp.NewPanelBase(nil, primitive, box),
	}
}

package ui

import (
	"github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/packages/appconfig"
	"github.com/rivo/tview"
)

type projectBasePanel struct {
	project appconfig.ProjectConfig
	tapp.PanelBase
}

func newProjectBasePanel(project appconfig.ProjectConfig, primitive tview.Primitive, box *tview.Box) projectBasePanel {
	return projectBasePanel{
		project:   project,
		PanelBase: tapp.NewPanelBase(nil, primitive, box),
	}
}

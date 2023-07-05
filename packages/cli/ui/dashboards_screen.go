package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
)

type dashboardsScreen struct {
	tapp.ScreenBase
}

func newDashboardsScreen(tui *tapp.TUI, project config.ProjectConfig) tapp.Screen {
	main := newDashboardsPanel(tui, project)

	return &dashboardsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenDashboards, main),
	}
}

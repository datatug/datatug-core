package ui

import (
	"github.com/datatug/datatug/apps/datatug/config"
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
)

type dashboardsScreen struct {
	tapp2.ScreenBase
}

func newDashboardsScreen(tui *tapp2.TUI, project config.ProjectConfig) tapp2.Screen {
	main := newDashboardsPanel(project)

	sidebar := newDashboardsSidebar(tui)

	return &dashboardsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenDashboards, main, sidebar),
	}
}

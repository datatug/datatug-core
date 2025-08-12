package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/packages/appconfig"
)

type dashboardsScreen struct {
	tapp2.ScreenBase
}

func newDashboardsScreen(tui *tapp2.TUI, project appconfig.ProjectConfig) tapp2.Screen {
	main := newDashboardsPanel(project)

	sidebar := newDashboardsSidebar(tui)

	return &dashboardsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenDashboards, main, sidebar),
	}
}

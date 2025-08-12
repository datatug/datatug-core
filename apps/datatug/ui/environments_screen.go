package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/packages/appconfig"
)

type environmentsScreen struct {
	tapp2.ScreenBase
}

func newEnvironmentsScreen(tui *tapp2.TUI, project appconfig.ProjectConfig) tapp2.Screen {

	main := newEnvironmentsPanel(project)

	sidebar := newProjectsMenu(tui)

	return &environmentsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenEnvironments, main, sidebar),
	}
}

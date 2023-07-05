package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
)

type environmentsScreen struct {
	tapp.ScreenBase
}

func newEnvironmentsScreen(tui *tapp.TUI, project config.ProjectConfig) tapp.Screen {

	main := newEnvironmentsPanel(project)

	sidebar := newProjectsMenu(tui)

	return &environmentsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenEnvironments, main, sidebar),
	}
}

package ui

import (
	"github.com/datatug/datatug/apps/datatugcli/config"
	tapp2 "github.com/datatug/datatug/apps/datatugcli/tapp"
)

type environmentsScreen struct {
	tapp2.ScreenBase
}

func newEnvironmentsScreen(tui *tapp2.TUI, project config.ProjectConfig) tapp2.Screen {

	main := newEnvironmentsPanel(project)

	sidebar := newProjectsMenu(tui)

	return &environmentsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenEnvironments, main, sidebar),
	}
}

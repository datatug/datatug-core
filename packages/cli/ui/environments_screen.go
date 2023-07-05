package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
)

type environmentsScreen struct {
	tapp.ScreenBase
}

func newEnvironmentsScreen(tui *tapp.TUI, project config.ProjectConfig) tapp.Screen {

	main := newEnvironmentsPanel(tui, project)

	return &environmentsScreen{
		ScreenBase: newProjectRootScreenBase(tui, project, ProjectScreenEnvironments, main),
	}
}

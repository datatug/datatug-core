package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
)

func newProjectScreen(tui *tapp.TUI, project config.ProjectConfig) tapp.Screen {
	return newEnvironmentsScreen(tui, project)
}

package ui

import (
	"github.com/datatug/datatug/apps/datatug/config"
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
)

func newProjectScreen(tui *tapp2.TUI, project config.ProjectConfig) tapp2.Screen {
	return newEnvironmentsScreen(tui, project)
}

package ui

import "github.com/datatug/datatug/packages/cli/tapp"

func NewHomeScreen(tui *tapp.TUI) tapp.Screen {
	return newProjectsScreen(tui)
}

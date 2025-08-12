package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatugcli/tapp"
)

func NewHomeScreen(tui *tapp2.TUI) tapp2.Screen {
	return newProjectsScreen(tui)
}

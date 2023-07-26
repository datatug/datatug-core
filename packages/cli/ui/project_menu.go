package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/rivo/tview"
)

type ProjectScreenID string

const (
	ProjectScreenDashboards   = "dashboards"
	ProjectScreenEnvironments = "environments"
)

func newProjectMenu(tui *tapp.TUI, project config.ProjectConfig, currentScreen ProjectScreenID) *projectMenu {
	list := tview.NewList().
		//AddItem("Databases", "", 'D', nil).
		AddItem("Dashboards", "", 'B', func() {
			tui.SetRootScreen(newDashboardsScreen(tui, project))
		}).
		AddItem("Environments", "", 'E', func() {
			tui.SetRootScreen(newEnvironmentsScreen(tui, project))
		})

	//AddItem("Queries", "", 'Q', nil).
	//AddItem("Web UI", "", 'W', nil)

	currentItem := -1
	switch currentScreen {
	case ProjectScreenDashboards:
		currentItem = 0
	case ProjectScreenEnvironments:
		currentItem = 1
	}
	if currentItem >= 0 {
		list.SetCurrentItem(currentItem)
	}

	defaultListStyle(list)

	return &projectMenu{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
	}
}

type projectMenu struct {
	tapp.PanelBase
}

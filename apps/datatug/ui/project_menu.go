package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/packages/appconfig"
	"github.com/rivo/tview"
)

type ProjectScreenID string

const (
	ProjectScreenDashboards   = "dashboards"
	ProjectScreenEnvironments = "environments"
)

func newProjectMenu(tui *tapp2.TUI, project appconfig.ProjectConfig, currentScreen ProjectScreenID) *projectMenu {
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
		PanelBase: tapp2.NewPanelBase(tui, list, list.Box),
	}
}

type projectMenu struct {
	tapp2.PanelBase
}

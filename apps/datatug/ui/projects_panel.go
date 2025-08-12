package ui

import (
	tapp2 "github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/packages/appconfig"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sort"
	"strconv"
)

var _ tview.Primitive = (*projectsPanel)(nil)
var _ tapp2.Cell = (*projectsPanel)(nil)

type projectsPanel struct {
	tapp2.PanelBase
	projects        []*appconfig.ProjectConfig
	selectProjectID string
	list            *tview.List
}

func newProjectsPanel(tui *tapp2.TUI) (*projectsPanel, error) {
	list := tview.NewList()
	panel := &projectsPanel{
		PanelBase: tapp2.NewPanelBase(tui, list, list.Box),
		list:      list,
	}

	settings, err := appconfig.GetSettings()
	if err != nil {
		return nil, err
	}

	openProject := func(projectConfig appconfig.ProjectConfig) {
		projectScreen := newProjectScreen(tui, projectConfig)
		tui.PushScreen(projectScreen)
	}

	panel.projects = settings.Projects

	sort.Slice(panel.projects, func(i, j int) bool {
		return panel.projects[i].ID < panel.projects[j].ID
	})

	projectSelected := func(p *appconfig.ProjectConfig) {
		panel.selectProjectID = p.ID
		openProject(*p)
	}
	for i, p := range panel.projects {
		project := p
		list.AddItem(project.ID, project.Url, rune(strconv.Itoa(i + 1)[0]), func() {
			projectSelected(project)
		})
	}

	list.SetTitle(" Projects") // TODO(ask-stackoverflow): how to set title?
	list.SetTitleColor(tview.Styles.TitleColor)

	defaultListStyle(list)

	list.SetTitleAlign(tview.AlignLeft)

	return panel, nil
}

func (p *projectsPanel) Draw(screen tcell.Screen) {
	var selectedItem = -1

	for i, proj := range p.projects {
		if proj.ID == p.selectProjectID {
			selectedItem = i
		}
	}
	if selectedItem >= 0 {
		p.list.SetCurrentItem(selectedItem)
	}
	p.list.Draw(screen)
}

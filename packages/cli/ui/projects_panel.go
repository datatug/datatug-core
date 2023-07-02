package ui

import (
	"github.com/datatug/datatug/packages/cli/config"
	"github.com/datatug/datatug/packages/cli/tapp"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"sort"
	"strconv"
)

var _ tview.Primitive = (*projectsPanel)(nil)
var _ tapp.Cell = (*projectsPanel)(nil)

type projectsPanel struct {
	tapp.PanelBase
	projects        []config.ProjectConfig
	selectProjectID string
	list            *tview.List
}

func newProjectsPanel(tui *tapp.TUI) (*projectsPanel, error) {
	list := tview.NewList()
	panel := &projectsPanel{
		PanelBase: tapp.NewPanelBase(tui, list, list.Box),
		list:      list,
	}

	settings, err := config.GetSettings()
	if err != nil {
		return nil, err
	}

	openProject := func(projectConfig config.ProjectConfig) {
		projectScreen := NewProjectScreen(tui, projectConfig)
		tui.PushScreen(projectScreen)
	}

	panel.projects = make([]config.ProjectConfig, 0, len(settings.Projects))

	for _, p := range settings.Projects { // Map to slice
		panel.projects = append(panel.projects, p)
	}
	sort.Slice(panel.projects, func(i, j int) bool {
		return panel.projects[i].ID < panel.projects[j].ID
	})

	projectSelected := func(p config.ProjectConfig) {
		panel.selectProjectID = p.ID
		openProject(p)
	}
	for i, p := range panel.projects {
		project := p
		list.AddItem(project.ID, project.Path, rune(strconv.Itoa(i + 1)[0]), func() {
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

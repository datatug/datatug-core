package uimodels

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps"
	"github.com/datatug/datatug/packages/bubbles"
	"strings"
)

type viewersModel struct {
	list   list.Model
	parent tea.Model
}

func (v viewersModel) Init() tea.Cmd {
	return nil
}

func (v viewersModel) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		v.list.SetSize(mm.Width, mm.Height)
		return v, nil
	case tea.KeyMsg:
		switch mm.Type {
		case tea.KeyEsc, tea.KeyBackspace:
			return v.parent, nil
		}
		switch s := strings.ToLower(mm.String()); s {
		case apps.QuitHotKey:
			return v, tea.Quit
		}
	}
	var err error
	if v.list, cmd = v.list.Update(msg); err != nil {
		fmt.Println("Failed to update menu list:", err)
	}
	return v, cmd
}

func (v viewersModel) View() string {
	return v.list.View()
}

func newViewersModel(parent tea.Model) *viewersModel {
	items := []list.Item{
		bubbles.NewMenuItem("Firestore viewer",
			"Browse & edit data in Firestore databases",
			bubbles.WithHotkey('I'),
		),
		bubbles.NewMenuItem("Files viewer",
			"Browse local files",
			bubbles.WithHotkey('I'),
		),
	}
	m := &viewersModel{
		list:   list.New(items, list.NewDefaultDelegate(), 30, 20),
		parent: parent,
	}
	m.list.ShowTitle()
	m.list.Title = "DataTug > Viewers"
	return m
}

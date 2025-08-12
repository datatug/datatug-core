package uimodels

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var _ list.Item = (*menuItem)(nil)

type menuItem struct {
	title       string
	description string
}

func (m menuItem) FilterValue() string {
	return m.title
}

func (m menuItem) Title() string {
	return m.title
}

func (m menuItem) Description() string {
	return m.description
}

type datatugMainMenu struct {
	list list.Model
}

func (m *datatugMainMenu) Init() tea.Cmd {
	return nil
}

func (m *datatugMainMenu) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(mm.Width, mm.Height)
		return m, nil
	case tea.KeyMsg:
		if mm.String() == "q" || mm.String() == "Q" {
			return m, tea.Quit
		}
	}
	var err error
	if m.list, cmd = m.list.Update(msg); err != nil {
		fmt.Println("Failed to update menu list:", err)
	}
	return m, cmd
}

func (m *datatugMainMenu) View() string {
	return m.list.View()
}

func DatatugMainMenu() tea.Model {
	items := []list.Item{
		menuItem{
			title:       "Sign in",
			description: "Authenticate to get more features",
		},
		menuItem{
			title:       "About",
			description: "Learn about this application",
		},
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 18)
	mainMenu := &datatugMainMenu{
		list: l,
	}
	// Configure the list on the model field (not the local copy),
	// because list.New returns a struct by value.
	mainMenu.list.Title = "Datatug Main Menu"
	mainMenu.list.SetShowStatusBar(false)
	mainMenu.list.SetFilteringEnabled(true)
	mainMenu.list.SetShowHelp(true)
	mainMenu.list.DisableQuitKeybindings() // prevent Esc/q from quitting the program
	mainMenu.list.Styles.Title = lipgloss.NewStyle().Bold(true)
	return mainMenu
}

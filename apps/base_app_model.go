package apps

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datatug/datatug/packages/bubbles/panel"
	"strings"
)

type BaseAppModel struct {
	Panels []panel.Panel
	//
	currentPanel int
}

func (m BaseAppModel) Init() tea.Cmd {
	return nil
}

func (m BaseAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		mm.Width = mm.Width / 2 // we have 2 Panels
	case tea.KeyMsg:
		switch mm.Type {
		case tea.KeyTab:
			if m.currentPanel < len(m.Panels)-1 {
				m.currentPanel++
			} else {
				m.currentPanel = 0
			}
			for i, p := range m.Panels {
				if i == m.currentPanel {
					p.Focus()
				} else {
					p.Blur()
				}
			}
		default:
			switch s := strings.ToLower(mm.String()); s {
			case QuitHotKey:
				return m, tea.Quit
			}
		}
	}
	pnl, cmd := m.Panels[m.currentPanel].Update(msg)
	m.Panels[m.currentPanel] = pnl.(panel.Panel)
	return m, cmd
}

func (m BaseAppModel) View() string {
	panels := make([]string, len(m.Panels))
	for i, p := range m.Panels {
		panels[i] = p.View()
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, panels...)
}

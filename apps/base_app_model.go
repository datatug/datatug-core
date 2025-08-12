package apps

import tea "github.com/charmbracelet/bubbletea"

type BaseAppModel struct {
	NavStack Stack[tea.Model]
}

func (m BaseAppModel) Init() tea.Cmd {
	return nil
}

func (m BaseAppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.NavStack.Current().Update(msg)
}

func (m BaseAppModel) View() string {
	return m.NavStack.Current().View()
}

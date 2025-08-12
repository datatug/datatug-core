package apps

import tea "github.com/charmbracelet/bubbletea"

type BaseAppModel struct {
	NavStack Stack[tea.Model]
}

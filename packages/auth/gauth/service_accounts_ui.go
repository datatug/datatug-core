package gauth

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// serviceAccountsUI is a child model for managing service accounts.
// Keys:
// - Enter on list: select current (no-op for now besides exiting)
// - a: add new (opens form)
// - ctrl+d: delete selected
// - Esc: go back (handled by parent)

type serviceAccountsUI struct {
	store Store

	items []list.Item
	child tea.Model

	status string
}

// saItem implements list.Item for service accounts.

type saItem struct{ ServiceAccountDbo }

func (i saItem) Title() string       { return i.Name }
func (i saItem) Description() string { return i.Path }
func (i saItem) FilterValue() string { return i.Name + " " + i.Path }

// control messages emitted by child models
type saOpenAddFormMsg struct{}
type saOpenAccountMenuMsg struct{ Account ServiceAccountDbo }
type saDeleteAccountMsg struct{ Account ServiceAccountDbo }

type saItemsUpdatedMsg struct{ Items []list.Item }

type serviceAccountsList struct {
	list  list.Model
	items []list.Item
}

func newServiceAccountsList(items []list.Item) tea.Model {
	l := list.New(items, list.NewDefaultDelegate(), 60, 18)
	l.Title = "Service accounts"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.DisableQuitKeybindings()
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	return &serviceAccountsList{list: l, items: items}
}

func (m *serviceAccountsList) Validate() error { return nil }
func (m *serviceAccountsList) Init() tea.Cmd   { return nil }

func (m *serviceAccountsList) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(mm.Width, mm.Height)
		return m, nil
	case tea.KeyMsg:
		switch {
		case mm.String() == "a":
			return m, func() tea.Msg { return saOpenAddFormMsg{} }
		case mm.Type == tea.KeyCtrlD:
			if it, ok := m.list.SelectedItem().(saItem); ok {
				acc := it.ServiceAccountDbo
				return m, func() tea.Msg { return saDeleteAccountMsg{Account: acc} }
			}
			return m, nil
		case mm.Type == tea.KeyEnter:
			if it, ok := m.list.SelectedItem().(saItem); ok {
				acc := it.ServiceAccountDbo
				return m, func() tea.Msg { return saOpenAccountMenuMsg{Account: acc} }
			}
			return m, nil
		case mm.Type == tea.KeyEsc:
			// No-op; parent handles navigation; also list quit is disabled
			return m, nil
		}
	case saItemsUpdatedMsg:
		m.items = mm.Items
		m.list.SetItems(mm.Items)
		return m, nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *serviceAccountsList) View() string {
	return m.list.View() + "\n[a] add  [ctrl+d] delete  [Enter] select  [Esc] back"
}

func NewServiceAccountsUI(store Store, accs []ServiceAccountDbo) (tea.Model, error) {
	it := make([]list.Item, 0, len(accs))
	for _, a := range accs {
		it = append(it, saItem{a})
	}
	return &serviceAccountsUI{
		store: store,
		items: it,
		child: newServiceAccountsList(it),
	}, nil
}

func (m *serviceAccountsUI) Init() tea.Cmd { return nil }

func (m *serviceAccountsUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle messages from the add form globally
	switch mm := msg.(type) {
	case AddServiceAccountSubmittedMsg:
		if err := m.appendAccount(mm.Account); err != nil {
			m.status = "Error: " + err.Error()
		}
		m.child = newServiceAccountsList(m.items)
		return m, func() tea.Msg { return ServiceAccountsUpdatedMsg(m.currentList()) }
	case AddServiceAccountCanceledMsg:
		m.child = newServiceAccountsList(m.items)
		return m, nil
	case tea.KeyMsg:
		// When in account menu, Esc returns to list
		if _, ok := m.child.(*serviceAccountMenu); ok && mm.Type == tea.KeyEsc {
			m.child = newServiceAccountsList(m.items)
			return m, nil
		}
	}

	if m.child == nil {
		m.child = newServiceAccountsList(m.items)
	}
	cm, cmd := m.child.Update(msg)
	m.child = cm

	// React to control messages from child
	switch mm := msg.(type) {
	case saOpenAddFormMsg:
		m.child = newAddServiceAccountForm()
		return m, nil
	case saOpenAccountMenuMsg:
		m.child = newServiceAccountMenu(mm.Account)
		return m, nil
	case saDeleteAccountMsg:
		m.deleteAccount(mm.Account)
		m.child = newServiceAccountsList(m.items)
		return m, func() tea.Msg { return ServiceAccountsUpdatedMsg(m.currentList()) }
	}
	return m, cmd
}

func (m *serviceAccountsUI) View() string {
	if m.child != nil {
		return m.child.View()
	}
	return ""
}

func (m *serviceAccountsUI) appendAccount(acc ServiceAccountDbo) error {
	// load existing, append, save
	accList, err := m.store.Load()
	if err != nil {
		return err
	}
	accList = append(accList, acc)
	if err := m.store.Save(accList); err != nil {
		return err
	}
	m.items = append(m.items, saItem{acc})
	return nil
}

func (m *serviceAccountsUI) deleteAccount(acc ServiceAccountDbo) {
	accList, err := m.store.Load()
	if err != nil {
		m.status = "Error: " + err.Error()
		return
	}
	newList := make([]ServiceAccountDbo, 0, len(accList))
	for _, a := range accList {
		if a.Name == acc.Name && a.Path == acc.Path {
			continue
		}
		newList = append(newList, a)
	}
	if err := m.store.Save(newList); err != nil {
		m.status = "Error: " + err.Error()
		return
	}
	// update view items
	newItems := make([]list.Item, 0, len(m.items))
	for _, it := range m.items {
		if si, ok := it.(saItem); ok {
			if si.Name == acc.Name && si.Path == acc.Path {
				continue
			}
		}
		newItems = append(newItems, it)
	}
	m.items = newItems
}

func (m *serviceAccountsUI) currentList() []ServiceAccountDbo {
	items := make([]ServiceAccountDbo, 0, len(m.items))
	for _, it := range m.items {
		items = append(items, it.(saItem).ServiceAccountDbo)
	}
	return items
}

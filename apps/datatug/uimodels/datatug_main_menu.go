package uimodels

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datatug/datatug/packages/bubbles"
	"strings"
)

func newDatatugMainMenu() tea.Model {
	items := []list.Item{
		bubbles.NewMenuItem(
			"Sign in [S]",
			"Authenticate for enabling collaboration and ability to save projects to DataTug cloud",
			bubbles.WithHotkey('S'),
		),
		bubbles.NewMenuItem("Projects",
			"You can store projects: locally, GitHub repo, DataTug cloud",
			bubbles.WithHotkey('P'),
		),
		bubbles.NewMenuItem("Viewers",
			"Utils for browsing & editing various data sources, like: Firestore, SQL db, etc.",
			bubbles.WithHotkey('V'),
		),
		bubbles.NewMenuItem("About",
			"Learn about this application",
			bubbles.WithHotkey('A'),
		),
		bubbles.NewMenuItem("Exit",
			"Quit the app",
			bubbles.WithHotkey('Q'),
		),
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 18)

	mainMenu := &datatugMainMenu{
		list: l,
	}
	// Configure the list on the model field (not the local copy),
	// because list.New returns a struct by value.
	//mainMenu.list.Styles.Title = lipgloss.NewStyle()
	//mainMenu.list.Title = "Datatug — Main Menu"
	mainMenu.list.SetShowTitle(false)
	mainMenu.list.SetShowStatusBar(false)
	mainMenu.list.SetFilteringEnabled(true)
	mainMenu.list.SetShowHelp(true)
	mainMenu.list.DisableQuitKeybindings() // prevent Esc/q from quitting the program
	mainMenu.list.Styles.Title = lipgloss.NewStyle().Bold(true)
	return mainMenu
}

type datatugMainMenu struct {
	list list.Model
}

func (m *datatugMainMenu) Init() tea.Cmd {
	return nil
}

func getMenuItemIndexByHotkey(l list.Model, hotKey string) int {
	for i, item := range l.Items() {
		if strings.HasSuffix(item.(list.DefaultItem).Title(), fmt.Sprintf("[%s]", hotKey)) {
			return i
		}
	}
	return -1
}

func (m *datatugMainMenu) Update(msg tea.Msg) (model tea.Model, cmd tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(mm.Width, mm.Height)
		return m, nil
	case tea.KeyMsg:
		switch mm.Type {
		case tea.KeyEnter:
			if it := m.list.SelectedItem(); it != nil {
				if mi, ok := it.(list.DefaultItem); ok {
					switch mi.Title() {
					case "Exit [Q]":
						return m, tea.Quit
					case "Viewers [V]":
						return m, func() tea.Msg {
							return bubbles.PushModel(newViewersModel(m))
						}
					}
				}
			}
		default:
			switch s := strings.ToLower(mm.String()); s {
			case "p", "v", "a", "s":
				if i := getMenuItemIndexByHotkey(m.list, strings.ToUpper(s)); i >= 0 {
					m.list.Select(i)
					return m.Update(tea.KeyMsg{Type: tea.KeyEnter})
				}
			}
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

//// Base style for the hotkey (underline + special colour)
//var unselectedHotkeyStyle = lipgloss.NewStyle().
//	Foreground(lipgloss.Color("11")). // Yellow
//	Underline(true)
//
//var selectedHotkeyStyle = lipgloss.NewStyle().
//	Foreground(lipgloss.Color("170")).
//	Underline(true)
//
//// Style for the whole line if selected
//var selectedStyle = lipgloss.NewStyle().
//	Foreground(lipgloss.Color("170")).
//	Bold(true)
//
//// Style for the whole line if selected
//var unselectedItemStyle = lipgloss.NewStyle().
//	Foreground(lipgloss.Color("240")) // dim gray

//type menuListDelegate struct {
//}
//
//var defaultMenuListDelegate = func() list.DefaultDelegate {
//	d := list.NewDefaultDelegate()
//	d.Styles.DimmedTitle = d.Styles.DimmedTitle.Underline(true)
//	d.Styles.NormalTitle = d.Styles.NormalTitle.Underline(true)
//	return d
//}()
//
//func (d menuListDelegate) Height() int                             { return 1 }
//func (d menuListDelegate) Spacing() int                            { return 0 }
//func (d menuListDelegate) Update(_ tea.Msg, _ *list.Model) tea.Cmd { return nil }
//func (d menuListDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
//	mi := listItem.(menuItem)
//	if mi.hotkey == 0 {
//		defaultMenuListDelegate.Render(w, m, index, listItem)
//		return
//	}
//	hotIndex := strings.Index(mi.title, string(mi.hotkey))
//
//	if hotIndex == -1 {
//		defaultMenuListDelegate.Render(w, m, index, listItem)
//		return
//	}
//
//	var normalStyle, hotKeyStyle lipgloss.Style
//
//	if m.SelectedItem() == listItem {
//		normalStyle = selectedStyle
//		hotKeyStyle = selectedHotkeyStyle
//	} else {
//		normalStyle = unselectedItemStyle
//		hotKeyStyle = unselectedHotkeyStyle
//	}
//
//	// Text before hotkey
//	if hotIndex > 0 {
//		_, _ = fmt.Fprintf(w, normalStyle.Render(mi.title[:hotIndex]))
//	}
//
//	// Hotkey letter with underline & colour
//	_, _ = fmt.Fprintf(w, hotKeyStyle.Render(string(mi.title[hotIndex])))
//
//	// Rest of the text
//	if hotIndex+1 < len(mi.title) {
//		_, _ = fmt.Fprintf(w, normalStyle.Render(mi.title[hotIndex+1:]))
//	}
//}

package fsviewer

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datatug/datatug/apps"
	"github.com/datatug/datatug/packages/auth/gauth"
	"github.com/pkg/browser"
	"log"
	"strings"
)

// App is the root Bubble Tea model for the Firestore Viewer.
// It presents a top menu and navigates to sub-models for Service Accounts, etc.

type App struct {
	apps.BaseAppModel
	activeService *gauth.ServiceAccountDbo
	saCount       int

	mode  appMode
	menu  list.Model
	child tea.Model // current child model (e.g., service accounts UI)
}

type appMode int

const (
	modeMainMenu appMode = iota
	modeServiceAccounts
	modeServiceAccountMenu
)

// NewApp constructs a new app and loads initial state.
func NewApp() (*App, error) {
	path, err := gauth.DefaultFilepath()
	if err != nil {
		return nil, err
	}
	store := gauth.FileStore{Filepath: path}
	accs, err := store.Load()
	if err != nil {
		return nil, err
	}
	app := &App{
		saCount: len(accs),
		mode:    modeMainMenu,
	}
	app.initMenu()
	return app, nil
}

func (a *App) initMenu() {
	items := a.topMenuItems()
	// Initialize with a sensible non-zero size so items are visible on first frame
	// even before the first WindowSizeMsg arrives. It will be overridden on resize.
	l := list.New(items, list.NewDefaultDelegate(), 60, 18)
	l.Title = "Firestore Viewer"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(false)
	l.SetShowHelp(true)
	l.DisableQuitKeybindings() // prevent Esc/q from quitting the program
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	a.menu = l
}

func (a *App) topMenuItems() []list.Item {
	label := fmt.Sprintf("Service accounts: %d", a.saCount)
	if a.activeService != nil {
		name := a.activeService.Name
		if name != "" {
			label = fmt.Sprintf("%s, active: %s", label, name)
		}
	}
	return []list.Item{
		menuItem{title: label, ID: "service_accounts", description: "List or edit Firebase service accounts"},
		menuItem{title: "About Firestore Viewer", ID: "about"},
	}
}

// Init implements tea.Model.
func (a *App) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Global messages from children
	switch m := msg.(type) {
	case gauth.ServiceAccountsUpdatedMsg:
		a.saCount = len(m)
		// refresh top menu label when in main menu
		a.menu.SetItems(a.topMenuItems())
	}

	switch a.mode {
	case modeMainMenu:
		return a.updateMainMenu(msg)
	case modeServiceAccounts, modeServiceAccountMenu:
		if current := a.NavStack.Current(); current != nil {
			m, cmd := current.Update(msg)
			a.NavStack.SetRoot(m)
			// listen for messages from child
			switch mm := msg.(type) {
			case tea.KeyMsg:
				if mm.Type == tea.KeyEsc {
					// Back to main menu
					a.mode = modeMainMenu
					a.NavStack.SetRoot(nil)
					// Refresh top menu counts
					a.menu.SetItems(a.topMenuItems())
				}
			}
			// Handle custom messages emitted by child
			return a, cmd
		}
	}
	return a, nil
}

func (a *App) updateMainMenu(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		// grow list to window
		a.menu.SetSize(mm.Width, mm.Height)
	case tea.KeyMsg:
		// In main menu allow q/Q to quit the program explicitly (we disabled list's default quit bindings)
		switch mm.Type {
		case tea.KeyEnter:
			if it, ok := a.menu.SelectedItem().(menuItem); ok {
				switch it.ID {
				case "service_accounts":
					// open service accounts child UI (create store and load accounts on demand)
					path, err := gauth.DefaultFilepath()
					if err != nil {
						log.Println("default filepath:", err)
						break
					}
					store := gauth.FileStore{Filepath: path}
					accs, err := store.Load()
					if err != nil {
						log.Println("load service accounts:", err)
						break
					}
					child, err := gauth.NewServiceAccountsUI(store, accs)
					if err != nil {
						log.Println("init service accounts UI:", err)
						break
					}
					a.NavStack.Push(child)
					a.mode = modeServiceAccounts
					return a, nil
				case "collections":
					// TODO: future
				case "about":
					// Open the project page in the default browser
					return a, func() tea.Msg {
						_ = browser.OpenURL("https://github.com/datatug/firestore-viewer")
						return nil
					}
				}
			}
		default:
			switch s := strings.ToLower(mm.String()); s {
			case apps.QuitHotKey:
				return a, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	a.menu, cmd = a.menu.Update(msg)
	return a, cmd
}

// View implements tea.Model.
func (a *App) View() string {
	switch a.mode {
	case modeMainMenu:
		return a.menu.View()
	default:
		if current := a.NavStack.Current(); current != nil {
			return current.View()
		}
		return ""
	}
}

type menuItem struct {
	title       string
	description string
	ID          string
}

func (m menuItem) Title() string       { return m.title }
func (m menuItem) Description() string { return m.description }
func (m menuItem) FilterValue() string { return m.title }

// itemDelegate is a simple list item renderer.

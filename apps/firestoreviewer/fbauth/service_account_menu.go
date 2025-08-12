package fbauth

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	firebase "google.golang.org/api/firebase/v1beta1"
)

type serviceAccountMenu struct {
	acc  ServiceAccountDbo
	list list.Model
}

// newServiceAccountMenu creates a list of Firebase projects available to the service account.
// For now, we derive the project list from the service account JSON (field project_id),
// which provides at least one project ID without external API calls.
func newServiceAccountMenu(acc ServiceAccountDbo) tea.Model {
	items := []list.Item{}
	if projects, err := projectsFromServiceAccount(acc.Path); err == nil {
		for _, project := range projects {
			items = append(items, menuItem{title: project.Name, id: project.ProjectId})
		}
	} else {
		// Show an inline error entry to inform the user, but keep navigation intact.
		items = append(items, menuItem{title: "Error loading projects", description: err.Error()})
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 18)
	l.Title = fmt.Sprintf("%s — Projects", acc.Name)
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	l.DisableQuitKeybindings() // prevent Esc from quitting; parent handles Esc to go back
	l.Styles.Title = lipgloss.NewStyle().Bold(true)
	return &serviceAccountMenu{acc: acc, list: l}
}

func (m *serviceAccountMenu) Validate() error { return nil }
func (m *serviceAccountMenu) Init() tea.Cmd   { return nil }

func (m *serviceAccountMenu) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(mm.Width, mm.Height)
		return m, nil
	case tea.KeyMsg:
		// Let parent handle Esc navigation; ensure list doesn't quit.
		if mm.Type == tea.KeyEsc {
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *serviceAccountMenu) View() string {
	return m.list.View() + "\n[Esc] back"
}

// projectsFromServiceAccount reads the credentials JSON and extracts project IDs.
// For tests and offline mode, we only parse the local JSON and return a single
// firebase.FirebaseProject constructed from the project_id field if present.
func projectsFromServiceAccount(path string) (projects []*firebase.FirebaseProject, err error) {
	if path == "" {
		return nil, fmt.Errorf("service account path is empty")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service account file: %w", err)
	}

	// Try two formats:
	// 1) Standard service account JSON with project_id field.
	// 2) Impersonated service account JSON with service_account_impersonation_url pointing to an email like name@project-id.iam.gserviceaccount.com
	var payload struct {
		ProjectID        string `json:"project_id"`
		Type             string `json:"type"`
		ImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if err = json.Unmarshal(b, &payload); err != nil {
		return nil, fmt.Errorf("parse service account json: %w", err)
	}

	projectID := strings.TrimSpace(payload.ProjectID)
	if projectID == "" && strings.TrimSpace(payload.ImpersonationURL) != "" {
		var u *url.URL
		// Parse from impersonation URL
		u, err = url.Parse(payload.ImpersonationURL)
		if err == nil {
			// Path like: /v1/projects/-/serviceAccounts/{email}:generateAccessToken
			p := u.Path
			idx := strings.Index(p, "/serviceAccounts/")
			if idx >= 0 {
				rest := p[idx+len("/serviceAccounts/"):]
				if colon := strings.Index(rest, ":"); colon >= 0 {
					rest = rest[:colon]
				}
				email, err := url.PathUnescape(rest)
				if err == nil {
					// email format: name@project-id.iam.gserviceaccount.com
					if at := strings.LastIndex(email, "@"); at >= 0 {
						domain := email[at+1:]
						const suffix = ".iam.gserviceaccount.com"
						if strings.HasSuffix(domain, suffix) {
							projectID = strings.TrimSuffix(domain, suffix)
						}
					}
				}
			}
		}
	}

	if projectID == "" {
		// No project_id derivable offline; return empty slice
		return []*firebase.FirebaseProject{}, nil
	}

	proj := &firebase.FirebaseProject{ProjectId: projectID, Name: projectID}
	return []*firebase.FirebaseProject{proj}, nil
}

package fbauth

import (
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// AddServiceAccountSubmittedMsg is emitted by the form when user confirms submission.
// The payload is the validated ServiceAccountDbo to be persisted by a parent model.
// It is defined in this file to keep coupling low.
type AddServiceAccountSubmittedMsg struct{ Account ServiceAccountDbo }

// AddServiceAccountCanceledMsg is emitted when the user cancels the form.
type AddServiceAccountCanceledMsg struct{}

// AddServiceAccountForm is a focused Bubble Tea model responsible for collecting
// a ServiceAccountDbo (Name + Path). It owns its own filepicker and text inputs.
// Navigation:
// - Tab: move focus to next field (Path -> Name -> Path ...).
// - Shift+Tab: move focus to previous field (Name -> Path -> Name ...).
// - Enter: if Path is focused and empty, opens the file picker; otherwise submits.
// - Esc in picker returns to form; Esc in form emits cancel.
//
// View is intentionally minimal; styling is left to parent context.

// Ensure each struct implements Validate() error per project style.

type addServiceAccountForm struct {
	nameInput textinput.Model
	pathInput textinput.Model
	picker    filepicker.Model

	mode addSAFormMode

	status string
}

type addSAFormMode int

const (
	addSAFormModeForm addSAFormMode = iota
	addSAFormModePick
)

func newAddServiceAccountForm() *addServiceAccountForm {
	name := textinput.New()
	name.Placeholder = "Account name"
	name.CharLimit = 128
	name.Prompt = "Name: "
	name.Width = 40 // sensible default so placeholder is fully visible

	path := textinput.New()
	path.Placeholder = "Path to JSON credentials"
	path.Prompt = "Path: "
	path.Width = 60 // wider, paths can be long
	path.Focus()    // Path should be the first field and focused by default

	picker := filepicker.New()
	picker.CurrentDirectory = "/"
	picker.AllowedTypes = []string{".json"}
	picker.SetHeight(12)

	return &addServiceAccountForm{
		nameInput: name,
		pathInput: path,
		picker:    picker,
		mode:      addSAFormModeForm,
	}
}

// Validate implements the convention-required method. For a UI form, there is
// no complex invariant; return nil to satisfy the interface.
func (f *addServiceAccountForm) Validate() error { return nil }

func (f *addServiceAccountForm) Init() tea.Cmd { return nil }

func (f *addServiceAccountForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Adjust widths on window resize so placeholders and input are visible
	switch wm := msg.(type) {
	case tea.WindowSizeMsg:
		// Leave some padding for prompts and borders; clamp to reasonable bounds
		w := wm.Width - 10
		if w < 20 {
			w = 20
		}
		if w > 100 {
			w = 100
		}
		f.nameInput.Width = w / 2
		if f.nameInput.Width < 30 {
			f.nameInput.Width = 30
		}
		f.pathInput.Width = w
	}

	switch f.mode {
	case addSAFormModeForm:
		return f.updateForm(msg)
	case addSAFormModePick:
		return f.updatePicker(msg)
	}
	return f, nil
}

func (f *addServiceAccountForm) View() string {
	switch f.mode {
	case addSAFormModeForm:
		return "Add service account\n\n" + f.pathInput.View() + "  (press Enter to open file picker)\n" + f.nameInput.View() + "\n\n[Enter] continue  [Esc] cancel"
	case addSAFormModePick:
		return "Pick JSON credentials file\n\n" + f.picker.View()
	}
	return ""
}

func (f *addServiceAccountForm) updateForm(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.KeyMsg:
		switch {
		case m.String() == "tab":
			// Next field: Path -> Name -> Path ...
			if f.pathInput.Focused() {
				f.pathInput.Blur()
				f.nameInput.Focus()
			} else {
				f.nameInput.Blur()
				f.pathInput.Focus()
			}
			return f, nil
		case m.String() == "shift+tab":
			// Previous field: Name -> Path -> Name ...
			if f.nameInput.Focused() {
				f.nameInput.Blur()
				f.pathInput.Focus()
			} else {
				f.pathInput.Blur()
				f.nameInput.Focus()
			}
			return f, nil
		case m.Type == tea.KeyEsc:
			return f, func() tea.Msg { return AddServiceAccountCanceledMsg{} }
		case m.Type == tea.KeyEnter:
			// If focused on path and empty -> open picker
			if f.pathInput.Focused() && f.pathInput.Value() == "" {
				f.mode = addSAFormModePick
				return f, f.picker.Init()
			}
			acc := ServiceAccountDbo{Name: f.nameInput.Value(), Path: f.pathInput.Value()}
			if err := acc.Validate(); err != nil {
				f.status = "Error: " + err.Error()
				return f, nil
			}
			return f, func() tea.Msg { return AddServiceAccountSubmittedMsg{Account: acc} }
		}
	}
	var cmd tea.Cmd
	f.nameInput, _ = f.nameInput.Update(msg)
	f.pathInput, cmd = f.pathInput.Update(msg)
	return f, cmd
}

func (f *addServiceAccountForm) updatePicker(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	f.picker, cmd = f.picker.Update(msg)
	switch m := msg.(type) {
	case tea.KeyMsg:
		if m.Type == tea.KeyEsc {
			f.mode = addSAFormModeForm
			return f, nil
		}
	}
	if did, path := f.picker.DidSelectFile(msg); did {
		f.pathInput.SetValue(path)
		// Autofill Name from selected file if Name is empty
		if strings.TrimSpace(f.nameInput.Value()) == "" {
			base := filepath.Base(path)
			name := strings.TrimSuffix(base, filepath.Ext(base))
			f.nameInput.SetValue(name)
		}
		f.mode = addSAFormModeForm
		return f, nil
	}
	return f, cmd
}

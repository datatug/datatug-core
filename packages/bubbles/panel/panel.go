package panel

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datatug/datatug/packages/bubbles"
)

type Panel interface {
	tea.Model
	SetTitle(title string)
	Focus()
	Blur()
	SetRoot(root tea.Model)
	Push(model tea.Model)
}

func New(inner tea.Model, title string) Panel {
	p := &panelModel{
		title:        title,
		focusedStyle: lipgloss.NewStyle().Border(lipgloss.ThickBorder()),
		blurStyle:    lipgloss.NewStyle().Border(lipgloss.NormalBorder()),
	}
	p.Push(inner)
	return p
}

type panelModel struct {
	title     string
	isFocused bool
	bubbles.Stack[tea.Model]
	focusedStyle lipgloss.Style
	blurStyle    lipgloss.Style
}

func (p *panelModel) Init() tea.Cmd {
	current := p.Current()
	if current == nil {
		return nil
	}
	return current.Init()
}

func (p *panelModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	current := p.Current()
	switch mm := msg.(type) {
	case tea.WindowSizeMsg:
		// Adjust the size for the border and pass adjusted msg further
		adj := tea.WindowSizeMsg{Width: mm.Width - 2, Height: mm.Height - 2}
		msg = adj
	}
	if current == nil {
		return p, nil
	}
	_, cmd := current.Update(msg)
	if cmd == nil {
		switch mm := msg.(type) {
		case tea.KeyMsg:
			switch mm.Type {
			case tea.KeyEsc, tea.KeyBackspace:
				if p.Len() > 1 {
					_, _ = p.Pop()
					return p, nil
				}
			}
		}
	} else {
		msg = cmd()
		if model, ok := msg.(bubbles.PushModel); ok {
			p.Push(model)
		}
	}
	return p, cmd
}

func (p *panelModel) View() string {
	current := p.Current()
	if current == nil {
		return "Panel has no models to render"
	}
	content := current.View()
	// Ensure non-empty content so the border renders even on initial frames
	if content == "" {
		content = " "
	}
	if p.isFocused {
		return p.focusedStyle.Render(content)
	} else {
		return p.blurStyle.Render(content)
	}
}

func (p *panelModel) Focus() {
	p.isFocused = true
}

func (p *panelModel) Blur() {
	p.isFocused = false
}

func (p *panelModel) SetTitle(title string) {
	p.title = title
}

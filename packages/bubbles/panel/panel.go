package panel

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/datatug/datatug/packages/bubbles"
	"strings"
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
		focusedStyle: lipgloss.NewStyle().Border(lipgloss.NormalBorder()),
		// Set blur panel border color to dim grey
		blurStyle: lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("#696969")),
	}
	p.Push(inner)
	return p
}

type panelModel struct {
	title     string
	isFocused bool
	size      tea.WindowSizeMsg
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
		p.size = tea.WindowSizeMsg{Width: mm.Width - 2, Height: mm.Height - 2}
		msg = p.size
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
			model.Update(p.size)
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
	// Choose style depending on focus
	style := p.blurStyle
	if p.isFocused {
		style = p.focusedStyle
	}

	framed := style.Render(content)

	// If there's a title, render it into the top border line
	if p.title != "" {
		b := style.GetBorderStyle()
		// If no border style, just return framed as is
		if (b != lipgloss.Border{}) {
			lines := strings.Split(framed, "\n")
			if len(lines) > 0 {
				topLine := lines[0]
				// Determine border pieces with fallbacks to space
				left := b.TopLeft
				right := b.TopRight
				h := b.Top
				if left == "" {
					left = " "
				}
				if right == "" {
					right = " "
				}
				if h == "" {
					h = " "
				}

				// Visible width of the top line in runes
				// Note: borders here do not use ANSI colors in our usage, so len is acceptable
				lineWidth := len([]rune(topLine))
				innerWidth := lineWidth - len([]rune(left)) - len([]rune(right))
				if innerWidth > 0 {
					titleText := " " + p.title + " "
					titleRunes := []rune(titleText)
					if len(titleRunes) > innerWidth {
						// Truncate to fit
						titleRunes = titleRunes[:innerWidth]
					}
					fillCount := innerWidth - len(titleRunes)
					fill := ""
					if fillCount > 0 {
						fillRunes := []rune(h)
						if len(fillRunes) == 0 {
							fillRunes = []rune(" ")
						}
						// Repeat h until fillCount is met
						for i := 0; i < fillCount; i++ {
							fill += string(fillRunes[i%len(fillRunes)])
						}
					}
					lines[0] = left + string(titleRunes) + fill + right
					// Apply the same border color to the reconstructed top line so it matches the border
					lines[0] = lipgloss.NewStyle().Foreground(style.GetBorderTopForeground()).Render(lines[0])
				}
			}
			framed = strings.Join(lines, "\n")
		}
	}

	return framed
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

package bubbles

import (
	"fmt"
	"github.com/charmbracelet/bubbles/list"
)

type menuItem struct {
	title       string
	description string
	hotkey      rune
}

func (m menuItem) FilterValue() string {
	return m.title
}

func (m menuItem) Title() string {
	if m.hotkey == 0 {
		return m.title
	}
	return fmt.Sprintf("%s [%c]", m.title, m.hotkey)
}

func (m menuItem) Description() string {
	return m.description
}

func NewMenuItem(title, description string, options ...func(mi *menuItem)) list.DefaultItem {
	mi := menuItem{title: title, description: description}
	for _, option := range options {
		option(&mi)
	}
	return mi
}

func WithHotkey(hotkey rune) func(mi *menuItem) {
	return func(mi *menuItem) {
		mi.hotkey = hotkey
	}
}

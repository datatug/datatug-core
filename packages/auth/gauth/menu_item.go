package gauth

import "github.com/charmbracelet/bubbles/list"

var _ list.Item = (*menuItem)(nil)

// menuItem implements list.Item
type menuItem struct {
	id          string
	title       string
	description string
}

func (i menuItem) Title() string       { return i.title }
func (i menuItem) Description() string { return i.description }
func (i menuItem) FilterValue() string { return i.title }

package tapp

import (
	"github.com/rivo/tview"
)

type Panel interface {
	tview.Primitive
	Box() *tview.Box
	TakeFocus()
}

var _ Panel = (*PanelBase)(nil)
var _ Cell = (*PanelBase)(nil)

type PanelBase struct {
	tui *TUI
	tview.Primitive
	box *tview.Box
}

func (p PanelBase) Box() *tview.Box {
	return p.box
}

func (p PanelBase) TakeFocus() {
	p.tui.App.SetFocus(p.Primitive)
}

func NewPanelBase(tui *TUI, primitive tview.Primitive, box *tview.Box) PanelBase {
	if tui == nil {
		panic("tui is nil")
	}
	if primitive == nil {
		panic("primitive is nil")
	}
	if box == nil {
		panic("box is nil")
	}
	return PanelBase{tui: tui, Primitive: primitive, box: box}
}

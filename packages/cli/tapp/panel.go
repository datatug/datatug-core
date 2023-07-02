package tapp

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Panel interface {
	TakeFocus()
}

var _ Panel = (*PanelBase)(nil)
var _ Cell = (*PanelBase)(nil)

type PanelBase struct {
	tui *TUI
	tview.Primitive
}

func (p PanelBase) Box() tview.Primitive {
	return p.Primitive
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
	box.SetFocusFunc(func() {
		box.SetBorderAttributes(tcell.AttrNone)
	})
	box.SetBlurFunc(func() {
		box.SetBorderAttributes(tcell.AttrDim)
	})
	return PanelBase{tui: tui, Primitive: primitive}
}

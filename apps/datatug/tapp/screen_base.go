package tapp

import "github.com/rivo/tview"

func NewScreenBase(tui *TUI, primitive tview.Primitive, options ScreenOptions) ScreenBase {
	return ScreenBase{Tui: tui, options: options, Primitive: primitive}
}

var _ Screen = (*ScreenBase)(nil)

type ScreenBase struct {
	Tui     *TUI
	options ScreenOptions
	tview.Primitive
}

func (screen *ScreenBase) TakeFocus() {
	screen.Tui.App.SetFocus(screen.Primitive)
}

func (screen *ScreenBase) Options() ScreenOptions {
	return screen.options
}

func (screen *ScreenBase) Window() tview.Primitive {
	return screen.Primitive
}

func (screen *ScreenBase) Activate() error {
	screen.Tui.App.SetFocus(screen.Primitive)
	return nil
}

//
//func (screen *ScreenBase) IntoBackground() {
//}

func (screen *ScreenBase) Close() error {
	screen.Tui.PopScreen()
	return nil
}

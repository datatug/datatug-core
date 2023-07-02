package tapp

import "github.com/rivo/tview"

var _ Screen = (*ScreenBase)(nil)

func NewScreenBase(tui *TUI, window tview.Primitive, options ScreenOptions) ScreenBase {
	return ScreenBase{Tui: tui, options: options, window: window}
}

type ScreenBase struct {
	Tui     *TUI
	options ScreenOptions
	window  tview.Primitive
}

func (screen *ScreenBase) TakeFocus() {
	screen.Tui.App.SetFocus(screen.window)
}

func (screen *ScreenBase) Options() ScreenOptions {
	return screen.options
}

func (screen *ScreenBase) Window() tview.Primitive {
	return screen.window
}

func (screen *ScreenBase) Activate() error {
	screen.Tui.App.SetFocus(screen.window)
	return nil
}

//
//func (screen *ScreenBase) IntoBackground() {
//}

func (screen *ScreenBase) Close() error {
	screen.Tui.PopScreen()
	return nil
}

package tapp

import (
	"fmt"
	"github.com/rivo/tview"
)

func NewTUI(app *tview.Application) *TUI {
	return &TUI{
		App: app,
	}
}

type TUI struct {
	App   *tview.Application
	stack []Screen
}

func (tui *TUI) StackDepth() int {
	return len(tui.stack)
}

func (tui *TUI) SetRootScreen(screen Screen) {
	tui.stack = []Screen{screen}
	tui.App.SetRoot(screen.Window(), screen.Options().FullScreen())
	if err := screen.Activate(); err != nil {
		panic(fmt.Errorf("failed to activate screen: %w", err))
	}
}

func (tui *TUI) PushScreen(screen Screen) {
	tui.stack = append(tui.stack, screen)
	tui.App.SetRoot(screen.Window(), screen.Options().FullScreen())
}

func (tui *TUI) PopScreen() {
	for len(tui.stack) > 1 {
		currentScreen := tui.stack[len(tui.stack)-1]
		tui.stack = tui.stack[:len(tui.stack)-1]

		currentScreen = tui.stack[len(tui.stack)-1]
		window := currentScreen.Window()
		options := currentScreen.Options()
		tui.App.SetRoot(window, options.fullScreen)
	}
}

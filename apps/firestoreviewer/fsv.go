package firestoreviewer

import (
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/datatug/datatug/apps/firestoreviewer/fsviewer"
	"os"
)

func Run() {
	app, err := fsviewer.NewApp()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	p := tea.NewProgram(app, tea.WithAltScreen())
	if _, err = p.Run(); err != nil {
		// Ensure the error is printed to console explicitly
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}

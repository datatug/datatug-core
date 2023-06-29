package commands

import (
	"github.com/datatug/datatug/packages/cli/ui"
	"github.com/rivo/tview"
)

func init() {
	_, err := Parser.AddCommand("ui", "Starts UI", "", &uiCommand{})
	if err != nil {
		panic(err)
	}
}

type uiCommand struct {
}

func (v *uiCommand) Execute(_ []string) error {

	app := tview.NewApplication().EnableMouse(true)
	homeScreen := ui.NewHomeScreen(app)

	return app.SetRoot(homeScreen, true).
		Run()
}

package commands

import (
	"github.com/datatug/datatug/apps/datatugcli/tapp"
	"github.com/datatug/datatug/apps/datatugcli/ui"
	"github.com/rivo/tview"
)

func uiCommandArgs(p Parser) {
	_, err := p.AddCommand("ui", "Starts UI", "", &uiCommand{})
	if err != nil {
		panic(err)
	}
}

type uiCommand struct {
}

func (v *uiCommand) Execute(_ []string) error {

	app := tview.NewApplication().EnableMouse(true)
	tui := tapp.NewTUI(app)
	_ = ui.NewHomeScreen(tui)

	return app.Run()
}

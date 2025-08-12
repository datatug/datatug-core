package commands

import (
	"context"
	"github.com/datatug/datatug/apps/datatug/tapp"
	"github.com/datatug/datatug/apps/datatug/ui"
	"github.com/rivo/tview"
	cliv3 "github.com/urfave/cli/v3"
)

func uiCommandArgs() *cliv3.Command {
	return &cliv3.Command{
		Name:        "ui",
		Usage:       "Starts UI",
		Description: "",
		Action: func(ctx context.Context, c *cliv3.Command) error {
			v := &uiCommand{}
			return v.Execute(nil)
		},
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

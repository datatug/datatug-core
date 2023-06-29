package ui

import "github.com/rivo/tview"

func NewFooterPanel() (footer tview.Primitive) {
	footer = &footerPanel{
		Primitive: tview.NewTextView().
			SetTextAlign(tview.AlignCenter).
			SetText("Footer"),
	}
	return footer
}

type footerPanel struct {
	tview.Primitive
}

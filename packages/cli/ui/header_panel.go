package ui

import "github.com/rivo/tview"

func NewHeaderPanel(project string) (header tview.Primitive) {
	textView := tview.NewTextView().
		SetTextAlign(tview.AlignLeft)

	if project != "" {
		textView.SetText("Datatug > " + project)
	} else {
		textView.SetText("Datatug")
	}

	textView.SetBorderPadding(0, 0, 1, 1)

	header = &headerPanel{
		Primitive: textView,
	}
	return header
}

type headerPanel struct {
	tview.Primitive
}

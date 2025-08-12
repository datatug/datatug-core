package ui

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func defaultBorder(box *tview.Box) {
	box.SetBorder(true)
	box.SetBorderColor(tcell.ColorCornflowerBlue)
	box.SetBorderAttributes(tcell.AttrDim)
	box.SetBorderPadding(1, 0, 1, 1)
}

func defaultListStyle(list *tview.List) {
	list.SetWrapAround(false)
	defaultBorder(list.Box)
}

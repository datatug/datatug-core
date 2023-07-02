package tapp

import (
	tcell "github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func NewRow(app *tview.Application, cells ...Cell) *Row {
	row := &Row{
		app:   app,
		cells: cells,
	}
	for _, cell := range cells {
		cell.Box().Focus(func(p tview.Primitive) {
			for i, c := range cells {
				if c.Box() == cell.Box() {
					row.activeCell = i
					break
				}
			}
		})
	}
	row.setKeyboardCapture()
	return row
}

type Cell interface {
	Box() tview.Primitive
	TakeFocus()
}

type Row struct {
	app        *tview.Application
	cells      []Cell
	activeCell int
}

func (row *Row) setKeyboardCapture() {
	moveRight := func() {
		if row.activeCell < len(row.cells)-1 {
			row.activeCell++
			row.cells[row.activeCell].TakeFocus()
		}
	}
	moveLeft := func() {
		if row.activeCell > 0 {
			row.activeCell--
			row.cells[row.activeCell].TakeFocus()
		}
	}
	row.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyRight:
			moveRight()
			return nil
		case tcell.KeyLeft:
			moveLeft()
			return nil
		case tcell.KeyTab:
			if event.Modifiers()&tcell.ModShift == tcell.ModShift {
				moveLeft()
			} else {
				moveRight()
			}
		}
		return event
	})
}

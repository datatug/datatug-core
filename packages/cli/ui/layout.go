package ui

import (
	"github.com/rivo/tview"
)

func layoutGrid(header *headerPanel) *tview.Grid {

	footer := NewFooterPanel()

	grid := tview.NewGrid()

	grid. // Default grid settings
		SetRows(1, 0, 1).
		SetColumns(20, 0, 20).
		SetBorders(false)

	grid. // Adds header and footer to the grid.
		AddItem(header, 0, 0, 1, 3, 0, 0, false).
		AddItem(footer, 2, 0, 1, 3, 0, 0, false)

	return grid
}

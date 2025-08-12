package commands

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func showRecordsetInGrid(recordset models.Recordset) error {
	app := tview.NewApplication()
	table := tview.NewTable()
	table.SetBorders(true)
	table.SetBordersColor(tcell.ColorLightGrey)
	for i, col := range recordset.Columns {
		cell := tview.NewTableCell(col.Name)
		cell.SetTextColor(tcell.ColorGold)
		switch col.DbType { //
		case "int", "number":
			cell.SetAlign(tview.AlignRight)
		}

		table.SetCell(0, i, cell)
	}
	for r, row := range recordset.Rows {
		for c, value := range row {
			cell := tview.NewTableCell(fmt.Sprintf("%v", value))
			cell.SetAlign(tview.AlignLeft) // TODO: align right for numbers
			table.SetCell(r+1, c, cell)
		}

	}
	table.Select(0, 0).
		SetFixed(1, 0).
		SetDoneFunc(func(key tcell.Key) {
			if key == tcell.KeyEscape {
				app.Stop()
			}
			if key == tcell.KeyEnter {
				table.SetSelectable(true, true)
			}
		}).
		SetSelectedFunc(func(row int, column int) {
			table.GetCell(row, column).SetTextColor(tcell.ColorRed)
			table.SetSelectable(false, false)
		})
	if err := app.SetRoot(table, true).
		EnableMouse(true).
		Run(); err != nil {
		panic(err)
	}
	return nil
}

package schemer

import "io"

func ReadColumns(r ColumnsReader) (columns []Column, err error) {
	var col Column
	for {
		if col, err = r.NextColumn(); err != nil {
			if err == io.EOF {
				err = nil
				break
			}
			return
		}
		columns = append(columns, col)
	}
	return
}

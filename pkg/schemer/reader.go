package schemer

import (
	"context"
	"errors"
	"io"
)

func ReadColumns(ctx context.Context, r ColumnsReader) (columns []Column, err error) {
	var col Column
	for {
		if ctx != nil {
			if err = ctx.Err(); err != nil {
				return
			}
		}
		if col, err = r.NextColumn(); err != nil {
			if errors.Is(err, io.EOF) {
				err = nil
			}
			return
		}
		if ctx != nil {
			if err = ctx.Err(); err != nil {
				return
			}
		}
		columns = append(columns, col)
	}
}

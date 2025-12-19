package schemer

import (
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

type mockColumnsReader struct {
	columns []Column
	index   int
	err     error
}

func (m *mockColumnsReader) NextColumn() (Column, error) {
	if m.err != nil && m.index >= len(m.columns) {
		return Column{}, m.err
	}
	if m.index >= len(m.columns) {
		return Column{}, io.EOF
	}
	col := m.columns[m.index]
	m.index++
	return col, nil
}

func TestReadColumns(t *testing.T) {
	tests := []struct {
		name    string
		reader  *mockColumnsReader
		want    []Column
		wantErr bool
	}{
		{
			name: "success",
			reader: &mockColumnsReader{
				columns: []Column{
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c2"}}},
				},
			},
			want: []Column{
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c2"}}},
			},
		},
		{
			name:   "empty",
			reader: &mockColumnsReader{},
			want:   nil,
		},
		{
			name: "error",
			reader: &mockColumnsReader{
				columns: []Column{
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
				err: errors.New("test error"),
			},
			want: []Column{
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadColumns(tt.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadColumns() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadColumns() got = %v, want %v", got, tt.want)
			}
		})
	}
}

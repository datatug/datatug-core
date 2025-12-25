package schemer

import (
	"context"
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
	onNext  func(m *mockColumnsReader)
}

func (m *mockColumnsReader) NextColumn() (Column, error) {
	if m.onNext != nil {
		m.onNext(m)
	}
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
	ctxCancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		reader  *mockColumnsReader
		want    []Column
		wantErr bool
	}{
		{
			name: "success",
			ctx:  context.Background(),
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
			ctx:    context.Background(),
			reader: &mockColumnsReader{},
			want:   nil,
		},
		{
			name: "error",
			ctx:  context.Background(),
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
		{
			name: "deadline_immediate",
			ctx:  ctxCancelled,
			reader: &mockColumnsReader{
				columns: []Column{
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "nil_context",
			ctx:  nil,
			reader: &mockColumnsReader{
				columns: []Column{
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
			},
			want: []Column{
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
			},
		},
		{
			name: "cancelled_context",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			reader: &mockColumnsReader{
				columns: []Column{
					{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				},
			},
			want:    nil,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ReadColumns(tt.ctx, tt.reader)
			if (err != nil) != tt.wantErr {
				t.Errorf("ReadColumns() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("ReadColumns() got = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("deadline_after_first", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		reader := &mockColumnsReader{
			columns: []Column{
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
				{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c2"}}},
			},
			onNext: func(m *mockColumnsReader) {
				if m.index == 1 {
					cancel()
				}
			},
		}
		got, err := ReadColumns(ctx, reader)
		if err == nil {
			t.Errorf("ReadColumns() error = nil, want context error")
		}
		want := []Column{
			{TableRef: TableRef{TableName: "t1"}, ColumnInfo: datatug.ColumnInfo{DbColumnProps: datatug.DbColumnProps{Name: "c1"}}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("ReadColumns() got = %v, want %v", got, want)
		}
	})
}
